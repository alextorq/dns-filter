package inspect

import (
	"context"
	"errors"
	"testing"
	"time"

	create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	source_db "github.com/alextorq/dns-filter/source/db"
	collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	suggest_db "github.com/alextorq/dns-filter/suggest-to-block/db"
	inspect_db "github.com/alextorq/dns-filter/suggest-to-block/inspect/db"
	"github.com/alextorq/dns-filter/utils"
)

type silentLogger struct{}

func (silentLogger) Info(...any) {}
func (silentLogger) Error(error) {}

type fakeCandidateRepo struct {
	pick    []inspect_db.InspectCandidate
	pickErr error
	saved   map[string]string
	retried map[string]int
	dropped map[string]bool
}

func (f *fakeCandidateRepo) PickForInspection(time.Duration, int) ([]inspect_db.InspectCandidate, error) {
	return f.pick, f.pickErr
}
func (f *fakeCandidateRepo) SaveResult(domain, verdict string) error {
	f.saved[domain] = verdict
	return nil
}
func (f *fakeCandidateRepo) ScheduleRetry(domain string, _ time.Duration) error {
	f.retried[domain]++
	return nil
}
func (f *fakeCandidateRepo) Drop(domain string) error {
	f.dropped[domain] = true
	return nil
}
func (f *fakeCandidateRepo) QueueDepth() (int64, error) {
	return int64(len(f.pick)), nil
}

type fakeInspector struct {
	results map[string]Result
	errs    map[string]error
	calls   []string
}

func (f *fakeInspector) Inspect(_ context.Context, fqdn string) (Result, error) {
	f.calls = append(f.calls, fqdn)
	if e := f.errs[fqdn]; e != nil {
		return Result{}, e
	}
	return f.results[fqdn], nil
}

type fakeBlockRepo struct {
	existing map[string]bool // canonical domains already in the blocklist
	created  map[string]string
	reasons  map[string][]create_domain.Reason
}

func (f *fakeBlockRepo) DomainNotExist(domain string) bool { return !f.existing[domain] }
func (f *fakeBlockRepo) CreateDomain(domain, source string) error {
	f.created[domain] = source
	return nil
}
func (f *fakeBlockRepo) CreateDomainWithReasons(domain, source string, reasons []create_domain.Reason) error {
	f.created[domain] = source
	f.reasons[domain] = reasons
	return nil
}

type fakeGate struct {
	active bool
	err    error
}

func (f *fakeGate) IsActive(source_db.BlockListSource) (bool, error) { return f.active, f.err }

type fakeFilter struct{ rebuilt int }

func (f *fakeFilter) UpdateFromDb() error { f.rebuilt++; return nil }

type fakeSuggest struct {
	err     error
	upserts map[string][]suggest_db.SuggestBlockReason
	scores  map[string]int
}

func (f *fakeSuggest) UpsertWithInspect(domain string, score int, reasons []suggest_db.SuggestBlockReason) error {
	if f.err != nil {
		return f.err
	}
	f.upserts[domain] = reasons
	f.scores[domain] = score
	return nil
}

type workerFakes struct {
	repo  *fakeCandidateRepo
	insp  *fakeInspector
	block *fakeBlockRepo
	gate  *fakeGate
	filt  *fakeFilter
	sug   *fakeSuggest
}

func newTestWorker(t *testing.T) (*Worker, *workerFakes) {
	t.Helper()
	f := &workerFakes{
		repo:  &fakeCandidateRepo{saved: map[string]string{}, retried: map[string]int{}, dropped: map[string]bool{}},
		insp:  &fakeInspector{results: map[string]Result{}, errs: map[string]error{}},
		block: &fakeBlockRepo{existing: map[string]bool{}, created: map[string]string{}, reasons: map[string][]create_domain.Reason{}},
		gate:  &fakeGate{active: true},
		filt:  &fakeFilter{},
		sug:   &fakeSuggest{upserts: map[string][]suggest_db.SuggestBlockReason{}, scores: map[string]int{}},
	}
	w := NewWorker(f.repo, f.insp, f.block, f.sug, f.gate, f.filt, silentLogger{}, WorkerConfig{
		Budget:    10,
		Interval:  time.Hour,
		CacheTTL:  time.Hour,
		Backoff:   time.Minute,
		MaxErrors: 3,
	})
	w.sleep = func(context.Context, time.Duration) {} // no real waiting in tests
	return w, f
}

func cand(domain string, score int) inspect_db.InspectCandidate {
	return inspect_db.InspectCandidate{Domain: domain, LexicalScore: score, ReasonsJSON: `[{"code":"risky_tld"}]`}
}

func hasCode(reasons []suggest_db.SuggestBlockReason, code string) bool {
	for _, r := range reasons {
		if r.Code == code {
			return true
		}
	}
	return false
}

// Malicious + kill-switch ON → auto-block with merged lexical+inspect reasons,
// bloom rebuilt once, verdict cached. Not surfaced to the suggest list.
func TestWorker_Malicious_AutoBlocks(t *testing.T) {
	w, f := newTestWorker(t)
	f.repo.pick = []inspect_db.InspectCandidate{cand("evil.com", 12)}
	f.insp.results["evil.com"] = Result{
		Verdict: "malicious",
		Reasons: []collect.Reason{{Code: collect.CodeInspectVTMalicious, Match: "malicious=5"}},
	}

	w.RunOnce(context.Background())

	key := utils.CanonicalDomain("evil.com")
	if f.block.created[key] != source_db.SourceAutoBlocked.String() {
		t.Fatalf("expected %s auto-blocked with AutoBlocked source, got %q", key, f.block.created[key])
	}
	codes := f.block.reasons[key]
	var hasLexical, hasInspect bool
	for _, r := range codes {
		if r.Code == collect.CodeRiskyTLD {
			hasLexical = true
		}
		if r.Code == collect.CodeInspectVTMalicious {
			hasInspect = true
		}
	}
	if !hasLexical || !hasInspect {
		t.Errorf("block reasons must merge lexical+inspect, got %+v", codes)
	}
	if f.filt.rebuilt != 1 {
		t.Errorf("bloom must rebuild exactly once, got %d", f.filt.rebuilt)
	}
	if !f.repo.dropped["evil.com"] {
		t.Error("auto-blocked domain must be dropped from the inspect queue")
	}
	if len(f.sug.upserts) != 0 {
		t.Errorf("auto-blocked domain must not also be surfaced to suggest, got %+v", f.sug.upserts)
	}
}

// Auto-block followed by a rate-limit on the next candidate: the run stops, but
// the bloom rebuild that the auto-block earned must STILL happen (break, not
// return). The rate-limited candidate stays untouched.
func TestWorker_AutoBlockThenRateLimit_StillRebuilds(t *testing.T) {
	w, f := newTestWorker(t)
	f.repo.pick = []inspect_db.InspectCandidate{cand("evil.com", 12), cand("limited.com", 12)}
	f.insp.results["evil.com"] = Result{Verdict: "malicious"}
	f.insp.errs["limited.com"] = ErrRateLimited

	w.RunOnce(context.Background())

	if len(f.block.created) != 1 {
		t.Errorf("first candidate must be auto-blocked, got %+v", f.block.created)
	}
	if f.filt.rebuilt != 1 {
		t.Errorf("bloom must rebuild despite the later rate-limit, got %d", f.filt.rebuilt)
	}
	if !f.repo.dropped["evil.com"] {
		t.Error("auto-blocked domain must be dropped")
	}
	if f.repo.saved["limited.com"] != "" || f.repo.retried["limited.com"] != 0 || f.repo.dropped["limited.com"] {
		t.Error("rate-limited candidate must be left untouched")
	}
}

// Malicious verdict for a domain already in the blocklist (e.g. blocked by a
// source sync since it was queued): no rebuild, no save, just dropped from the
// queue. Exercises the autoBlockExists branch.
func TestWorker_Malicious_AlreadyBlocked_DropsNoRebuild(t *testing.T) {
	w, f := newTestWorker(t)
	f.block.existing[utils.CanonicalDomain("evil.com")] = true // create_domain → ErrDomainAlreadyExists
	f.repo.pick = []inspect_db.InspectCandidate{cand("evil.com", 12)}
	f.insp.results["evil.com"] = Result{Verdict: "malicious"}

	w.RunOnce(context.Background())

	if f.filt.rebuilt != 0 {
		t.Errorf("already-blocked domain must not trigger a rebuild, got %d", f.filt.rebuilt)
	}
	if !f.repo.dropped["evil.com"] {
		t.Error("already-blocked domain must be dropped from the queue")
	}
	if _, saved := f.repo.saved["evil.com"]; saved {
		t.Errorf("already-blocked domain must not be cached, saved=%q", f.repo.saved["evil.com"])
	}
}

// A failing suggest upsert is logged but must not crash the run, and the verdict
// is still cached (save is independent of surface).
func TestWorker_SurfaceError_DoesNotBreakRun(t *testing.T) {
	w, f := newTestWorker(t)
	f.sug.err = errors.New("suggest db down")
	f.repo.pick = []inspect_db.InspectCandidate{cand("fresh.com", 15)}
	f.insp.results["fresh.com"] = Result{Verdict: "suspicious"}

	w.RunOnce(context.Background()) // must not panic

	if f.repo.saved["fresh.com"] != "suspicious" {
		t.Errorf("verdict must still be cached despite surface error, saved=%q", f.repo.saved["fresh.com"])
	}
}

// Malicious + kill-switch OFF → surfaced to suggest, NOT blocked, no rebuild.
func TestWorker_Malicious_KillSwitchOff_Surfaces(t *testing.T) {
	w, f := newTestWorker(t)
	f.gate.active = false
	f.repo.pick = []inspect_db.InspectCandidate{cand("evil.com", 12)}
	f.insp.results["evil.com"] = Result{
		Verdict: "malicious",
		Reasons: []collect.Reason{{Code: collect.CodeInspectVTMalicious}},
	}

	w.RunOnce(context.Background())

	if len(f.block.created) != 0 {
		t.Errorf("kill-switch off: must not auto-block, got %+v", f.block.created)
	}
	if f.filt.rebuilt != 0 {
		t.Errorf("kill-switch off: bloom must not rebuild, got %d", f.filt.rebuilt)
	}
	reasons, ok := f.sug.upserts["evil.com"]
	if !ok {
		t.Fatal("malicious domain must be surfaced to suggest when auto-block is off")
	}
	if !hasCode(reasons, collect.CodeRiskyTLD) || !hasCode(reasons, collect.CodeInspectVTMalicious) {
		t.Errorf("surfaced reasons must merge lexical+inspect, got %+v", reasons)
	}
	if f.sug.scores["evil.com"] != 12 {
		t.Errorf("surfaced score must stay lexical (12), got %d", f.sug.scores["evil.com"])
	}
	if f.repo.saved["evil.com"] != "malicious" {
		t.Errorf("verdict cached = %q, want malicious", f.repo.saved["evil.com"])
	}
}

// Suspicious → surfaced to suggest, cached, never blocked.
func TestWorker_Suspicious_Surfaces(t *testing.T) {
	w, f := newTestWorker(t)
	f.repo.pick = []inspect_db.InspectCandidate{cand("fresh.com", 15)}
	f.insp.results["fresh.com"] = Result{
		Verdict: "suspicious",
		Reasons: []collect.Reason{{Code: collect.CodeInspectRDAPYoung, Match: "age_days=3"}},
	}

	w.RunOnce(context.Background())

	if _, ok := f.sug.upserts["fresh.com"]; !ok {
		t.Error("suspicious domain must be surfaced to suggest")
	}
	if len(f.block.created) != 0 {
		t.Errorf("suspicious must not be auto-blocked, got %+v", f.block.created)
	}
	if f.repo.saved["fresh.com"] != "suspicious" {
		t.Errorf("verdict cached = %q, want suspicious", f.repo.saved["fresh.com"])
	}
}

// Clean → verdict cached (NOT dropped, NOT surfaced, NOT retried) so it is not
// re-inspected until the TTL — protecting the quota.
func TestWorker_Clean_CachesVerdict(t *testing.T) {
	w, f := newTestWorker(t)
	f.repo.pick = []inspect_db.InspectCandidate{cand("legit.com", 12)}
	f.insp.results["legit.com"] = Result{Verdict: "clean"}

	w.RunOnce(context.Background())

	if f.repo.saved["legit.com"] != "clean" {
		t.Errorf("clean verdict must be cached, saved=%q", f.repo.saved["legit.com"])
	}
	if len(f.sug.upserts) != 0 || len(f.block.created) != 0 {
		t.Errorf("clean must not surface or block; suggest=%+v block=%+v", f.sug.upserts, f.block.created)
	}
	if f.repo.retried["legit.com"] != 0 {
		t.Errorf("clean must not be retried, got %d", f.repo.retried["legit.com"])
	}
}

// Unknown with retries left → ScheduleRetry, no terminal verdict. Unknown with
// the error budget exhausted → cache "unknown" so it stops consuming budget.
func TestWorker_Unknown_RetriesThenGivesUp(t *testing.T) {
	w, f := newTestWorker(t) // MaxErrors = 3

	fresh := cand("a.com", 12)     // ErrorCount 0 → retry
	exhausted := cand("b.com", 12) // ErrorCount 2 → 2+1>=3 → give up
	exhausted.ErrorCount = 2
	f.repo.pick = []inspect_db.InspectCandidate{fresh, exhausted}
	f.insp.results["a.com"] = Result{Verdict: "unknown"}
	f.insp.results["b.com"] = Result{Verdict: "unknown"}

	w.RunOnce(context.Background())

	if f.repo.retried["a.com"] != 1 {
		t.Errorf("fresh unknown must be retried once, got %d", f.repo.retried["a.com"])
	}
	if _, saved := f.repo.saved["a.com"]; saved {
		t.Errorf("fresh unknown must not be cached as terminal, got %q", f.repo.saved["a.com"])
	}
	if f.repo.saved["b.com"] != "unknown" {
		t.Errorf("exhausted unknown must be cached, got %q", f.repo.saved["b.com"])
	}
	if f.repo.retried["b.com"] != 0 {
		t.Errorf("exhausted unknown must not be retried again, got %d", f.repo.retried["b.com"])
	}
}

// Rate-limit short-circuits the run: the offending candidate is left UNTOUCHED
// (no save, no retry) and subsequent candidates are not inspected at all.
func TestWorker_RateLimited_StopsRunUntouched(t *testing.T) {
	w, f := newTestWorker(t)
	f.repo.pick = []inspect_db.InspectCandidate{cand("a.com", 12), cand("b.com", 12)}
	f.insp.errs["a.com"] = ErrRateLimited
	f.insp.results["b.com"] = Result{Verdict: "malicious"}

	w.RunOnce(context.Background())

	if len(f.insp.calls) != 1 || f.insp.calls[0] != "a.com" {
		t.Errorf("run must stop after rate-limit; inspected %v, want [a.com]", f.insp.calls)
	}
	if _, saved := f.repo.saved["a.com"]; saved {
		t.Error("rate-limited candidate must not be saved")
	}
	if f.repo.retried["a.com"] != 0 {
		t.Error("rate-limited candidate must not be scheduled for retry")
	}
	if len(f.block.created) != 0 || len(f.sug.upserts) != 0 {
		t.Error("no candidate after rate-limit should be acted on")
	}
}

// A transient (non-rate-limit) inspect error retries the domain and the run
// continues to the next candidate.
func TestWorker_InspectError_RetriesAndContinues(t *testing.T) {
	w, f := newTestWorker(t)
	f.repo.pick = []inspect_db.InspectCandidate{cand("a.com", 12), cand("b.com", 12)}
	f.insp.errs["a.com"] = errors.New("network blip")
	f.insp.results["b.com"] = Result{Verdict: "clean"}

	w.RunOnce(context.Background())

	if f.repo.retried["a.com"] != 1 {
		t.Errorf("errored domain must be retried, got %d", f.repo.retried["a.com"])
	}
	if f.repo.saved["b.com"] != "clean" {
		t.Errorf("run must continue past the error; b.com saved=%q", f.repo.saved["b.com"])
	}
}

// Kill-switch query failure must fail closed: a malicious verdict is surfaced
// to suggest, never auto-blocked.
func TestWorker_KillSwitchError_FailsClosed(t *testing.T) {
	w, f := newTestWorker(t)
	f.gate.err = errors.New("sources table gone")
	f.repo.pick = []inspect_db.InspectCandidate{cand("evil.com", 12)}
	f.insp.results["evil.com"] = Result{Verdict: "malicious"}

	w.RunOnce(context.Background())

	if len(f.block.created) != 0 {
		t.Errorf("fail-closed: must not auto-block on kill-switch error, got %+v", f.block.created)
	}
	if _, ok := f.sug.upserts["evil.com"]; !ok {
		t.Error("fail-closed: malicious domain must fall through to suggest")
	}
}

// A PickForInspection error is logged and the run is a no-op — never a panic.
func TestWorker_PickError_NoPanic(t *testing.T) {
	w, f := newTestWorker(t)
	f.repo.pickErr = errors.New("db down")

	w.RunOnce(context.Background())

	if len(f.insp.calls) != 0 {
		t.Error("pick error must abort the run before any inspection")
	}
}

// A cancelled context stops the run before inspecting anything.
func TestWorker_CtxCancelled_StopsEarly(t *testing.T) {
	w, f := newTestWorker(t)
	f.repo.pick = []inspect_db.InspectCandidate{cand("a.com", 12), cand("b.com", 12)}
	f.insp.results["a.com"] = Result{Verdict: "clean"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.RunOnce(ctx)

	if len(f.insp.calls) != 0 {
		t.Errorf("cancelled ctx must stop before inspection, inspected %v", f.insp.calls)
	}
}
