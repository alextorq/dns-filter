// Package suggest_to_block is the composition root for the suggest-to-block
// feature. Module orchestrates Collect (read blocked + allowed lists, score
// candidates, optionally auto-promote, persist the rest) on a 12h ticker.
package suggest_to_block

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	source_db "github.com/alextorq/dns-filter/source/db"
	collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
)

// BlockRepo is the narrow port over the blocklist this module needs. It must
// satisfy create_domain.Repo for the auto-block path (CreateDomain checks
// DomainNotExist + writes), plus expose GetAllActiveURLs for the input set.
type BlockRepo interface {
	GetAllActiveURLs() ([]string, error)
	DomainNotExist(domain string) bool
	CreateDomain(domain, source string) error
	CreateDomainWithReasons(domain, source string, reasons []create_domain.Reason) error
}

type AllowRepo interface {
	GetAllActiveFilters() ([]string, error)
}

type SourceGate interface {
	IsActive(name source_db.BlockListSource) (bool, error)
}

// Filter abstracts filter.Module — Collect rebuilds the bloom only after at
// least one auto-block lands.
type Filter interface {
	UpdateFromDb() error
}

type SuggestRepo interface {
	CreateBatch(suggests []suggest_to_block_db.SuggestBlock) error
	GetByFilter(params suggest_to_block_db.GetAllParams) (*suggest_to_block_db.GetAllResult, error)
	UpdateActive(id uint, active bool) error
}

// InspectQueue is the optional sink for weak lexical candidates — those scoring
// in [collect.MinInspectCandidateScore, collect.ThresholdToSuggestBlocking).
// When wired (the opt-in reputation feature is enabled), Collect upserts them
// here for the inspect worker to enrich. When nil, Collect drops them exactly
// as the pre-refactor CollectSuggest filter did, so the feature stays inert.
// *inspect_db.Repo satisfies it.
type InspectQueue interface {
	UpsertCandidate(domain string, lexicalScore int, reasonsJSON string) error
}

type Logger interface {
	Info(args ...any)
	Error(err error)
}

type Module struct {
	blockRepo    BlockRepo
	allowRepo    AllowRepo
	sourceGate   SourceGate
	filter       Filter
	repo         SuggestRepo
	log          Logger
	inspectQueue InspectQueue // optional; nil keeps the feature inert
	// inspectGate — рантайм-гейт включения reputation-обогащения.
	// nil = всегда включено (обратная совместимость с тестами и со средой,
	// где SetInspectQueue не звался). В production main.go выставляет gate
	// в inspect.IsEnabled, чтобы UI-тогл управлял маршрутизацией без
	// рестарта.
	inspectGate func() bool
}

// SetInspectQueue wires the optional reputation-enrichment queue. Call once at
// composition time, before Start. Leaving it unset disables queue population —
// Collect then behaves exactly as before the feature existed.
func (m *Module) SetInspectQueue(q InspectQueue) { m.inspectQueue = q }

// SetInspectGate подключает рантайм-гейт включения inspect-обогащения. Когда
// gate возвращает false, Collect не маршрутизирует кандидатов в очередь и не
// обращается к InspectQueue вообще — поведение идентично "очередь не подключена".
func (m *Module) SetInspectGate(g func() bool) { m.inspectGate = g }

// inspectActive — true, когда очередь подключена и фича не выключена рантайм-
// гейтом. nil-gate = "всегда включено" (тестовое поведение).
func (m *Module) inspectActive() bool {
	if m.inspectQueue == nil {
		return false
	}
	if m.inspectGate == nil {
		return true
	}
	return m.inspectGate()
}

func NewModule(
	blockRepo BlockRepo,
	allowRepo AllowRepo,
	sourceGate SourceGate,
	filter Filter,
	repo SuggestRepo,
	log Logger,
) *Module {
	return &Module{
		blockRepo:  blockRepo,
		allowRepo:  allowRepo,
		sourceGate: sourceGate,
		filter:     filter,
		repo:       repo,
		log:        log,
	}
}

// Collect runs one batch: load blocked + allowed lists, score candidates,
// auto-block those that pass either gate (if the AutoBlocked source is
// enabled), persist the rest as suggestions, and rebuild the bloom only when
// at least one auto-block landed.
func (m *Module) Collect() error {
	m.log.Info("Start collecting suggestions to block domains")
	blocked, err := m.blockRepo.GetAllActiveURLs()
	if err != nil {
		return err
	}

	allowed, err := m.allowRepo.GetAllActiveFilters()
	if err != nil {
		return err
	}

	// When the operator has disabled the AutoBlocked source via the UI, Collect
	// must not write anything to block_lists — neither active nor inactive
	// rows. Such candidates fall through to the suggest queue for manual
	// review, mirroring how Sync() skips disabled sources entirely. We fail
	// closed on a DB error: silently auto-blocking when we can't confirm the
	// source is active would defeat the kill-switch.
	autoBlockEnabled, err := m.sourceGate.IsActive(source_db.SourceAutoBlocked)
	if err != nil {
		m.log.Error(err)
		autoBlockEnabled = false
	}

	// Score every candidate once, then bucket by score. The heavy blocked-index
	// scoring happens a single time inside ScoreCandidates; the thresholds here
	// are pure policy.
	scored := collect.ScoreCandidates(blocked, allowed)
	suggests := make([]suggest_to_block_db.SuggestBlock, 0, len(scored))
	var autoBlocked, autoAlreadyBlocked, autoErrors, queued int

	for _, domain := range scored {
		// Strong lexical signal: surface in the UI (and maybe auto-block),
		// reproducing the pre-refactor CollectSuggest(>=Threshold) behaviour.
		if domain.Score >= collect.ThresholdToSuggestBlocking {
			if autoBlockEnabled && collect.ShouldAutoBlock(domain) {
				switch m.autoBlock(domain) {
				case autoBlockInserted:
					autoBlocked++
				case autoBlockAlreadyExists:
					autoAlreadyBlocked++
				case autoBlockError:
					autoErrors++
				}
				continue
			}

			reasons := make([]suggest_to_block_db.SuggestBlockReason, 0, len(domain.Reasons))
			for _, r := range domain.Reasons {
				reasons = append(reasons, suggest_to_block_db.SuggestBlockReason{
					Code:       r.Code,
					MatchValue: r.Match,
				})
			}
			suggests = append(suggests, suggest_to_block_db.SuggestBlock{
				Domain:  domain.Domain,
				Score:   domain.Score,
				Reasons: reasons,
			})
			continue
		}

		// Weak lexical signal: too low for the UI on its own, but worth a
		// reputation check IF the feature is wired AND включена в БД-настройках.
		// Когда фича отключена (queue не подключена или gate=false), кандидат
		// отбрасывается — поведение идентично состоянию "очередь не подключена",
		// то есть кэш очереди не растёт впустую и не выплеснется внезапно при
		// последующем включении.
		if m.inspectActive() && domain.Score >= collect.MinInspectCandidateScore {
			if err := m.inspectQueue.UpsertCandidate(domain.Domain, domain.Score, reasonsJSON(domain.Reasons)); err != nil {
				m.log.Error(err)
			} else {
				queued++
			}
		}
	}

	if err := m.repo.CreateBatch(suggests); err != nil {
		return err
	}

	// Bloom filter must be rebuilt exactly once per Collect, after all auto-
	// promotions land in the DB — otherwise auto-blocked domains keep
	// resolving until the next manual mutation triggers a rebuild. We only
	// rebuild on *newly inserted* domains: already-blocked entries don't
	// change the bloom set, and pure errors produced nothing to rebuild for.
	if autoBlocked > 0 {
		if err := m.filter.UpdateFromDb(); err != nil {
			m.log.Error(err)
		}
	}

	m.log.Info(
		"Finished collecting suggestions to block domains;",
		"auto-blocked:", autoBlocked,
		"already-blocked:", autoAlreadyBlocked,
		"auto-errors:", autoErrors,
		"to-suggest:", len(suggests),
		"queued-for-inspect:", queued,
	)
	return nil
}

// reasonsJSON serialises a candidate's lexical reasons for the inspect queue's
// snapshot column, so the worker can later merge them with inspect_* reasons
// when promoting the domain. A marshal failure (not expected for these plain
// structs) degrades to an empty array rather than failing the whole batch.
func reasonsJSON(reasons []collect.Reason) string {
	// Normalise the empty case to "[]" rather than json.Marshal's "null" for a
	// nil slice, so the snapshot column always holds a valid JSON array.
	if len(reasons) == 0 {
		return "[]"
	}
	b, err := json.Marshal(reasons)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// autoBlockOutcome distinguishes the three terminal states of autoBlock so the
// caller can tally them separately. Plain bool was hiding "already blocked"
// inside "false" together with real errors — operators couldn't tell whether
// the run was a healthy idempotent re-run or a sea of DB failures.
type autoBlockOutcome int

const (
	autoBlockInserted autoBlockOutcome = iota
	autoBlockAlreadyExists
	autoBlockError
)

// autoBlock promotes a single Suggestion straight to the blocklist with
// SourceAutoBlocked. Errors are logged but never propagated — auto-block is
// best-effort and must not break the rest of the Collect batch.
func (m *Module) autoBlock(s collect.Suggestion) autoBlockOutcome {
	// Carry the reason codes into block_lists so the verdict survives in the
	// DB — the application log rotates and is not a durable audit trail (#95).
	reasons := make([]create_domain.Reason, 0, len(s.Reasons))
	codes := make([]string, 0, len(s.Reasons))
	for _, r := range s.Reasons {
		reasons = append(reasons, create_domain.Reason{Code: r.Code, Match: r.Match})
		codes = append(codes, r.Code)
	}

	err := create_domain.CreateDomain(
		create_domain.Deps{Repo: m.blockRepo, Log: m.log},
		create_domain.RequestBody{
			Domain:  s.Domain,
			Source:  source_db.SourceAutoBlocked.String(),
			Reasons: reasons,
		},
	)
	if errors.Is(err, create_domain.ErrDomainAlreadyExists) {
		return autoBlockAlreadyExists
	}
	if err != nil {
		m.log.Error(err)
		return autoBlockError
	}

	m.log.Info("Auto-blocked domain from suggest:", s.Domain, "score:", s.Score, "reasons:", codes)
	return autoBlockInserted
}

// Start runs Collect immediately and then on a 12h ticker until ctx is done.
// Block forever — call from a goroutine. ctx cancellation is the only way to
// stop the loop cleanly; per-tick errors are logged but do not break it.
func (m *Module) Start(ctx context.Context) {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	tryCollect := func() {
		if err := m.Collect(); err != nil {
			m.log.Error(err)
		}
	}

	tryCollect()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tryCollect()
		}
	}
}
