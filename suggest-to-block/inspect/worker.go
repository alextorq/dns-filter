package inspect

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
	source_db "github.com/alextorq/dns-filter/source/db"
	collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	suggest_db "github.com/alextorq/dns-filter/suggest-to-block/db"
	inspect_db "github.com/alextorq/dns-filter/suggest-to-block/inspect/db"
)

// Compile-time guarantees that the production types satisfy the worker ports,
// so a signature drift is caught here rather than at the main.go wiring site.
var (
	_ Inspector       = (*Adapter)(nil)
	_ CandidateRepo   = (*inspect_db.Repo)(nil)
	_ SuggestUpserter = (*suggest_db.Repo)(nil)
)

// Inspector is the reputation port — *Adapter satisfies it.
type Inspector interface {
	Inspect(ctx context.Context, fqdn string) (Result, error)
}

// CandidateRepo is the queue port — *inspect_db.Repo satisfies it.
type CandidateRepo interface {
	PickForInspection(ttl time.Duration, budget int) ([]inspect_db.InspectCandidate, error)
	SaveResult(domain, verdict string) error
	ScheduleRetry(domain string, backoff time.Duration) error
	Drop(domain string) error
	QueueDepth() (int64, error)
}

// SuggestUpserter promotes a domain into the suggest list — *suggest_db.Repo
// satisfies it.
type SuggestUpserter interface {
	UpsertWithInspect(domain string, lexicalScore int, reasons []suggest_db.SuggestBlockReason) error
}

// BlockRepo is the auto-block output port — *blocked-domain/db.Repo satisfies it
// (and create_domain.Repo).
type BlockRepo interface {
	DomainNotExist(domain string) bool
	CreateDomain(domain, source string) error
	CreateDomainWithReasons(domain, source string, reasons []create_domain.Reason) error
}

// SourceGate gates auto-block on the AutoBlocked source toggle (the kill-switch).
type SourceGate interface {
	IsActive(name source_db.BlockListSource) (bool, error)
}

type Filter interface {
	UpdateFromDb() error
}

type Logger interface {
	Info(args ...any)
	Error(err error)
}

// WorkerConfig holds the runtime knobs (wired from env in the composition root).
type WorkerConfig struct {
	Budget    int           // max domains inspected per tick (bounds VT quota use)
	Interval  time.Duration // tick period
	CacheTTL  time.Duration // re-inspect a domain only after this long
	Pause     time.Duration // delay between external calls (rate-limit pacing)
	Backoff   time.Duration // retry delay for transient failures
	MaxErrors int           // give up retrying after this many failures
}

// Worker drains the inspect queue: it picks the highest-scoring weak-lexical
// candidates, runs the reputation adapter against each (paced to respect the
// VirusTotal quota), and acts on the verdict — auto-block, surface to the
// suggest list, or cache the result so it is not re-checked until the TTL.
type Worker struct {
	repo      CandidateRepo
	inspector Inspector
	blockRepo BlockRepo
	suggest   SuggestUpserter
	gate      SourceGate
	filter    Filter
	log       Logger
	cfg       WorkerConfig
	// sleep is the pacing primitive, injectable so tests run instantly. The
	// default is ctx-aware so a pause aborts promptly on shutdown.
	sleep func(context.Context, time.Duration)
}

func NewWorker(
	repo CandidateRepo,
	inspector Inspector,
	blockRepo BlockRepo,
	suggest SuggestUpserter,
	gate SourceGate,
	filter Filter,
	log Logger,
	cfg WorkerConfig,
) *Worker {
	return &Worker{
		repo:      repo,
		inspector: inspector,
		blockRepo: blockRepo,
		suggest:   suggest,
		gate:      gate,
		filter:    filter,
		log:       log,
		cfg:       cfg,
		sleep:     ctxSleep,
	}
}

// Start runs one batch immediately, then on the configured ticker until ctx is
// done. Block forever — call from a goroutine. Mirrors suggest_to_block.Start;
// we do NOT use periodic.Run because it ignores context (no clean shutdown).
func (w *Worker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	w.RunOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

// RunOnce drains up to Budget candidates. It never returns an error — like the
// collector, a worker run must not crash a serving process; every failure is
// logged and the run continues (or pauses, for rate-limiting).
func (w *Worker) RunOnce(ctx context.Context) {
	if depth, err := w.repo.QueueDepth(); err == nil {
		inspectQueueDepth.Set(float64(depth))
	}

	candidates, err := w.repo.PickForInspection(w.cfg.CacheTTL, w.cfg.Budget)
	if err != nil {
		w.log.Error(err)
		return
	}
	if len(candidates) == 0 {
		return
	}

	// Kill-switch is read once per run: it gates whether a malicious verdict
	// auto-blocks or merely surfaces. Fail closed on a DB error — never silently
	// auto-block when we cannot confirm the source is enabled.
	autoBlockEnabled, err := w.gate.IsActive(source_db.SourceAutoBlocked)
	if err != nil {
		w.log.Error(err)
		autoBlockEnabled = false
	}

	var rebuildFilter bool
	for i, c := range candidates {
		if ctx.Err() != nil { // shutdown between domains
			return
		}
		if i > 0 {
			w.sleep(ctx, w.cfg.Pause) // pace external calls to stay under quota
		}

		result, err := w.inspector.Inspect(ctx, c.Domain)
		if errors.Is(err, ErrRateLimited) {
			// Quota exhausted for the whole provider — stop the run and leave
			// THIS candidate untouched (no SaveResult/ScheduleRetry): it is not a
			// per-domain failure, just "try again next tick".
			inspectRateLimited.Inc()
			w.log.Info("inspect: rate-limited, pausing run after", i, "domains")
			break
		}
		if err != nil {
			inspectErrors.Inc()
			w.log.Error(err)
			w.retryOrGiveUp(c)
			continue
		}
		inspectDecisions.WithLabelValues(result.Verdict).Inc()

		switch result.Verdict {
		case string(domain_inspect.VerdictMalicious):
			if autoBlockEnabled {
				switch w.autoBlock(c, result.Reasons) {
				case autoBlockInserted:
					rebuildFilter = true
					w.drop(c.Domain) // now permanently in the blocklist — leave the queue
				case autoBlockExists:
					w.drop(c.Domain) // already blocked — nothing more to inspect
				case autoBlockFailed:
					w.retryOrGiveUp(c) // transient write failure — try again next cycle
				}
			} else {
				// Kill-switch off: surface for manual review and cache the verdict
				// so it is not re-inspected/re-surfaced until the TTL.
				w.surface(c, result.Reasons)
				w.save(c.Domain, domain_inspect.VerdictMalicious)
			}
		case string(domain_inspect.VerdictSuspicious):
			w.surface(c, result.Reasons)
			w.save(c.Domain, domain_inspect.VerdictSuspicious)
		case string(domain_inspect.VerdictClean):
			// Cache the clean verdict instead of dropping the row: a dropped
			// candidate would be re-queued by the next Collect and re-inspected,
			// wasting quota on a domain we already cleared. PickForInspection
			// skips it until the TTL expires.
			w.save(c.Domain, domain_inspect.VerdictClean)
		default: // unknown — could not decide; retry a bounded number of times
			w.retryOrGiveUp(c)
		}
	}

	if rebuildFilter {
		if err := w.filter.UpdateFromDb(); err != nil {
			w.log.Error(err)
		}
	}
}

// retryOrGiveUp schedules a backoff retry, or caches an "unknown" verdict once
// the candidate has failed MaxErrors times so it stops consuming budget.
func (w *Worker) retryOrGiveUp(c inspect_db.InspectCandidate) {
	if c.ErrorCount+1 >= w.cfg.MaxErrors {
		w.save(c.Domain, domain_inspect.VerdictUnknown)
		return
	}
	if err := w.repo.ScheduleRetry(c.Domain, w.cfg.Backoff); err != nil {
		w.log.Error(err)
	}
}

func (w *Worker) save(domain string, verdict domain_inspect.Verdict) {
	if err := w.repo.SaveResult(domain, string(verdict)); err != nil {
		w.log.Error(err)
	}
}

func (w *Worker) drop(domain string) {
	if err := w.repo.Drop(domain); err != nil {
		w.log.Error(err)
	}
}

// autoBlockOutcome distinguishes the three terminal states of an auto-block so
// the caller can react: rebuild the bloom (inserted), just clean up the queue
// (already exists), or retry later (transient failure).
type autoBlockOutcome int

const (
	autoBlockInserted autoBlockOutcome = iota
	autoBlockExists
	autoBlockFailed
)

// autoBlock promotes the candidate straight into the blocklist with the
// AutoBlocked source, carrying both lexical and inspect reasons. Reuses the
// canonical create_domain path (which normalises the domain to its FQDN form);
// the candidate is stored by Collect in the matching trailing-dot-trimmed form,
// so create_domain's CanonicalDomain produces the key the blocklist uses.
func (w *Worker) autoBlock(c inspect_db.InspectCandidate, inspectReasons []collect.Reason) autoBlockOutcome {
	reasons := make([]create_domain.Reason, 0)
	for _, r := range lexicalReasons(c.ReasonsJSON) {
		reasons = append(reasons, create_domain.Reason{Code: r.Code, Match: r.Match})
	}
	for _, r := range inspectReasons {
		reasons = append(reasons, create_domain.Reason{Code: r.Code, Match: r.Match})
	}

	err := create_domain.CreateDomain(
		create_domain.Deps{Repo: w.blockRepo, Log: w.log},
		create_domain.RequestBody{
			Domain:  c.Domain,
			Source:  source_db.SourceAutoBlocked.String(),
			Reasons: reasons,
		},
	)
	if errors.Is(err, create_domain.ErrDomainAlreadyExists) {
		return autoBlockExists
	}
	if err != nil {
		w.log.Error(err)
		return autoBlockFailed
	}
	w.log.Info("inspect: auto-blocked", c.Domain)
	return autoBlockInserted
}

// surface upserts the candidate into the suggest list with its lexical + inspect
// reasons for manual review. Score stays lexical.
func (w *Worker) surface(c inspect_db.InspectCandidate, inspectReasons []collect.Reason) {
	reasons := make([]suggest_db.SuggestBlockReason, 0)
	for _, r := range lexicalReasons(c.ReasonsJSON) {
		reasons = append(reasons, suggest_db.SuggestBlockReason{Code: r.Code, MatchValue: r.Match})
	}
	for _, r := range inspectReasons {
		reasons = append(reasons, suggest_db.SuggestBlockReason{Code: r.Code, MatchValue: r.Match})
	}
	if err := w.suggest.UpsertWithInspect(c.Domain, c.LexicalScore, reasons); err != nil {
		w.log.Error(err)
	}
}

// lexicalReasons decodes the candidate's snapshot column. A malformed/empty
// snapshot degrades to no lexical reasons rather than failing the promotion.
func lexicalReasons(snapshot string) []collect.Reason {
	if snapshot == "" {
		return nil
	}
	var rs []collect.Reason
	if err := json.Unmarshal([]byte(snapshot), &rs); err != nil {
		return nil
	}
	return rs
}

// ctxSleep waits for d or until ctx is done, whichever comes first.
func ctxSleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
