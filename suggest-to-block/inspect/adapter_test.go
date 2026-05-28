package inspect

import (
	"context"
	"errors"
	"testing"
	"time"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
	collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	inspect_db "github.com/alextorq/dns-filter/suggest-to-block/inspect/db"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newRepo(t *testing.T) *inspect_db.Repo {
	t.Helper()
	conn, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := conn.AutoMigrate(&inspect_db.InspectCandidate{}, &inspect_db.RDAPCache{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return inspect_db.NewRepo(conn)
}

func fakeCheck(status domain_inspect.CheckStatus, verdict domain_inspect.Verdict, details map[string]any) domain_inspect.CheckFunc {
	return func(context.Context, string) domain_inspect.CheckResult {
		return domain_inspect.CheckResult{Status: status, Verdict: verdict, Details: details}
	}
}

func hasReason(reasons []collect.Reason, code string) (collect.Reason, bool) {
	for _, r := range reasons {
		if r.Code == code {
			return r, true
		}
	}
	return collect.Reason{}, false
}

// Two providers calling "malicious" push the summary over the malicious
// threshold; both must surface as distinct inspect_* reasons.
func TestInspect_Malicious(t *testing.T) {
	a := NewAdapter(newRepo(t), time.Hour)
	a.checks = map[string]domain_inspect.CheckFunc{
		"virustotal":    fakeCheck(domain_inspect.StatusOK, domain_inspect.VerdictMalicious, map[string]any{"malicious": 6}),
		"safe_browsing": fakeCheck(domain_inspect.StatusOK, domain_inspect.VerdictMalicious, nil),
	}

	res, err := a.Inspect(context.Background(), "evil.example.com")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if res.Verdict != "malicious" {
		t.Errorf("verdict = %q, want malicious", res.Verdict)
	}
	if r, ok := hasReason(res.Reasons, collect.CodeInspectVTMalicious); !ok {
		t.Error("missing inspect_vt_malicious reason")
	} else if r.Match != "malicious=6" {
		t.Errorf("vt match = %q, want malicious=6", r.Match)
	}
	if _, ok := hasReason(res.Reasons, collect.CodeInspectSafeBrowsing); !ok {
		t.Error("missing inspect_safe_browsing reason")
	}
}

// A rate-limited provider short-circuits the whole inspection so the worker can
// pause — even if another check already produced a verdict.
func TestInspect_RateLimitedShortCircuits(t *testing.T) {
	a := NewAdapter(newRepo(t), time.Hour)
	a.checks = map[string]domain_inspect.CheckFunc{
		"virustotal":    fakeCheck(domain_inspect.StatusRateLimited, "", nil),
		"safe_browsing": fakeCheck(domain_inspect.StatusOK, domain_inspect.VerdictMalicious, nil),
	}

	_, err := a.Inspect(context.Background(), "evil.example.com")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

// A young domain alone scores below the summary threshold (verdict stays
// "unknown"), but the inspect_rdap_young reason MUST still be recorded so the
// worker sees the signal. This pins the "reasons independent of summary" rule.
func TestInspect_YoungDomain_ReasonRecordedDespiteUnknownSummary(t *testing.T) {
	a := NewAdapter(newRepo(t), time.Hour)
	a.checks = map[string]domain_inspect.CheckFunc{
		"rdap": fakeCheck(domain_inspect.StatusOK, domain_inspect.VerdictSuspicious, map[string]any{"age_days": 5}),
	}

	res, err := a.Inspect(context.Background(), "fresh.example.com")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if res.Verdict != "unknown" {
		t.Errorf("a lone suspicious signal should summarize as unknown, got %q", res.Verdict)
	}
	if r, ok := hasReason(res.Reasons, collect.CodeInspectRDAPYoung); !ok {
		t.Error("inspect_rdap_young must be recorded even when summary is unknown")
	} else if r.Match != "age_days=5" {
		t.Errorf("rdap match = %q, want age_days=5", r.Match)
	}
}

// An actively-clean summary is recorded as an endorsement so the worker can
// drop the candidate rather than leave it pending.
func TestInspect_CleanEndorsed(t *testing.T) {
	a := NewAdapter(newRepo(t), time.Hour)
	a.checks = map[string]domain_inspect.CheckFunc{
		"safe_browsing": fakeCheck(domain_inspect.StatusOK, domain_inspect.VerdictClean, nil),
	}

	res, err := a.Inspect(context.Background(), "legit.example.com")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if res.Verdict != "clean" {
		t.Errorf("verdict = %q, want clean", res.Verdict)
	}
	if _, ok := hasReason(res.Reasons, collect.CodeInspectCleanEndorsed); !ok {
		t.Error("missing inspect_clean_endorsed reason")
	}
}

// When every provider is unavailable (no API keys → skipped, RDAP errored) the
// adapter must report an honest "unknown" with no reasons and no error. This is
// the steady state on a deployment without VT/SB keys, and the worker's
// retry-vs-drop branch in M4 depends on it: "unknown" means "could not decide",
// not "clean" and not a failure.
func TestInspect_AllNonOK_UnknownNoReasons(t *testing.T) {
	a := NewAdapter(newRepo(t), time.Hour)
	a.checks = map[string]domain_inspect.CheckFunc{
		"virustotal":    fakeCheck(domain_inspect.StatusSkipped, "", nil),
		"safe_browsing": fakeCheck(domain_inspect.StatusError, "", nil),
		"rdap":          fakeCheck(domain_inspect.StatusError, "", nil),
	}

	res, err := a.Inspect(context.Background(), "x.example.com")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if res.Verdict != "unknown" {
		t.Errorf("verdict = %q, want unknown", res.Verdict)
	}
	if len(res.Reasons) != 0 {
		t.Errorf("expected no reasons, got %+v", res.Reasons)
	}
}

// Sibling FQDNs under one registrable must share the RDAP result: the inner
// (network) check runs once, the second sibling is served from cache with the
// same verdict re-derived from the stored age.
func TestWithRDAPCache_SiblingServedFromCache(t *testing.T) {
	a := NewAdapter(newRepo(t), time.Hour)
	calls := 0
	inner := func(context.Context, string) domain_inspect.CheckResult {
		calls++
		return domain_inspect.CheckResult{
			Status:  domain_inspect.StatusOK,
			Verdict: domain_inspect.VerdictSuspicious,
			Details: map[string]any{"age_days": 3},
		}
	}
	wrapped := a.withRDAPCache(inner)

	first := wrapped(context.Background(), "a.evil.com")
	second := wrapped(context.Background(), "b.evil.com") // same eTLD+1 → cache hit

	if calls != 1 {
		t.Errorf("inner RDAP should run once for sibling FQDNs, ran %d times", calls)
	}
	if first.Verdict != domain_inspect.VerdictSuspicious || second.Verdict != domain_inspect.VerdictSuspicious {
		t.Errorf("verdicts: first=%s second=%s, want both suspicious", first.Verdict, second.Verdict)
	}
	if cached, _ := second.Details["cached"].(bool); !cached {
		t.Error("second result should be marked cached")
	}
	if age, _ := second.Details["age_days"].(int); age != 3 {
		t.Errorf("cached age = %v, want 3", second.Details["age_days"])
	}
}

// A well-aged domain served from cache must re-derive the "clean" verdict via
// RDAPVerdictForAge — exercising the >365d branch that the suspicious-age test
// does not.
func TestWithRDAPCache_CleanAgeFromCache(t *testing.T) {
	a := NewAdapter(newRepo(t), time.Hour)
	calls := 0
	inner := func(context.Context, string) domain_inspect.CheckResult {
		calls++
		return domain_inspect.CheckResult{
			Status:  domain_inspect.StatusOK,
			Verdict: domain_inspect.VerdictClean,
			Details: map[string]any{"age_days": 400},
		}
	}
	wrapped := a.withRDAPCache(inner)

	wrapped(context.Background(), "a.aged.com")
	second := wrapped(context.Background(), "b.aged.com")

	if calls != 1 {
		t.Errorf("inner should run once, ran %d", calls)
	}
	if second.Verdict != domain_inspect.VerdictClean {
		t.Errorf("cache-hit for age=400 should be clean, got %s", second.Verdict)
	}
}

// A non-OK inner result must NOT be cached: a transient RDAP error should be
// re-tried on the next sibling, not frozen as a bogus age.
func TestWithRDAPCache_InnerErrorNotCached(t *testing.T) {
	a := NewAdapter(newRepo(t), time.Hour)
	calls := 0
	inner := func(context.Context, string) domain_inspect.CheckResult {
		calls++
		return domain_inspect.CheckResult{Status: domain_inspect.StatusError, Error: "boom"}
	}
	wrapped := a.withRDAPCache(inner)

	wrapped(context.Background(), "a.err.com")
	wrapped(context.Background(), "b.err.com") // same registrable, but nothing cached

	if calls != 2 {
		t.Errorf("errored RDAP must not be cached, inner ran %d times, want 2", calls)
	}
}

// An OK result that carries no age_days (registrar 404 / unregistered) must not
// be cached as age=0 — otherwise siblings would inherit a false "young" verdict.
func TestWithRDAPCache_MissingAgeNotCached(t *testing.T) {
	a := NewAdapter(newRepo(t), time.Hour)
	calls := 0
	inner := func(context.Context, string) domain_inspect.CheckResult {
		calls++
		return domain_inspect.CheckResult{
			Status:  domain_inspect.StatusOK,
			Verdict: domain_inspect.VerdictUnknown,
			Details: map[string]any{"registered": false},
		}
	}
	wrapped := a.withRDAPCache(inner)

	wrapped(context.Background(), "a.noage.com")
	wrapped(context.Background(), "b.noage.com")

	if calls != 2 {
		t.Errorf("result without age_days must not be cached, inner ran %d times, want 2", calls)
	}
}

// A non-registrable input (bare TLD) is delegated to inner every time and never
// cached — otherwise we would poison the cache under a meaningless key.
func TestWithRDAPCache_NonRegistrableDelegatesUncached(t *testing.T) {
	a := NewAdapter(newRepo(t), time.Hour)
	calls := 0
	inner := func(context.Context, string) domain_inspect.CheckResult {
		calls++
		return domain_inspect.CheckResult{Status: domain_inspect.StatusOK, Verdict: domain_inspect.VerdictUnknown}
	}
	wrapped := a.withRDAPCache(inner)

	wrapped(context.Background(), "com")
	wrapped(context.Background(), "com")

	if calls != 2 {
		t.Errorf("non-registrable input must always delegate (no caching), inner ran %d times, want 2", calls)
	}
}
