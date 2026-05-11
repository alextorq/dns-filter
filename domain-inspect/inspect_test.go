package domain_inspect

import (
	"context"
	"strings"
	"testing"
	"time"
)

// fanout: every registered check must produce exactly one result, the names
// are preserved by the runner even if the check forgets to set them, and
// results come back sorted by name (stable contract for the UI/clients).
func TestInspect_FanoutAndOrdering(t *testing.T) {
	checks := map[string]CheckFunc{
		"zeta": func(_ context.Context, _ string) CheckResult {
			return CheckResult{Status: StatusOK, Verdict: VerdictClean}
		},
		"alpha": func(_ context.Context, _ string) CheckResult {
			return CheckResult{Status: StatusOK, Verdict: VerdictMalicious}
		},
		"mike": func(_ context.Context, _ string) CheckResult {
			return CheckResult{Status: StatusSkipped}
		},
	}

	res := Inspect(context.Background(), "example.com", checks)

	if res.Domain != "example.com" {
		t.Errorf("domain not propagated: got %q", res.Domain)
	}
	if len(res.Checks) != len(checks) {
		t.Fatalf("expected %d results, got %d", len(checks), len(res.Checks))
	}

	wantOrder := []string{"alpha", "mike", "zeta"}
	for i, want := range wantOrder {
		if res.Checks[i].Name != want {
			t.Errorf("result %d: expected name %q, got %q", i, want, res.Checks[i].Name)
		}
	}
}

// A check that forgets to set Name in the returned struct must still be named
// by the runner — otherwise a sloppy check author silently breaks the contract.
func TestInspect_ForcesNameFromKey(t *testing.T) {
	res := Inspect(context.Background(), "x", map[string]CheckFunc{
		"forgot-its-name": func(_ context.Context, _ string) CheckResult {
			return CheckResult{Status: StatusOK} // Name intentionally empty
		},
	})

	if len(res.Checks) != 1 || res.Checks[0].Name != "forgot-its-name" {
		t.Fatalf("runner must fill Name from map key; got %+v", res.Checks)
	}
}

// Recovery contract: one panicking check must not crash the whole inspection,
// and the failure must surface as a normal CheckResult with status=error.
func TestInspect_RecoversFromPanic(t *testing.T) {
	res := Inspect(context.Background(), "x", map[string]CheckFunc{
		"good": func(_ context.Context, _ string) CheckResult {
			return CheckResult{Status: StatusOK, Verdict: VerdictClean}
		},
		"boom": func(_ context.Context, _ string) CheckResult {
			panic("boom")
		},
	})

	if len(res.Checks) != 2 {
		t.Fatalf("expected 2 results despite panic, got %d", len(res.Checks))
	}

	var boom *CheckResult
	for i := range res.Checks {
		if res.Checks[i].Name == "boom" {
			boom = &res.Checks[i]
		}
	}
	if boom == nil {
		t.Fatal("missing 'boom' result")
	}
	if boom.Status != StatusError {
		t.Errorf("expected status=error for panic, got %s", boom.Status)
	}
	if !strings.Contains(boom.Error, "boom") {
		t.Errorf("expected panic value in error, got %q", boom.Error)
	}
}

// Checks should run concurrently — if Inspect serialized them, this test would
// take ~600ms. With concurrency it stays close to 200ms. We use a comfortable
// 500ms ceiling so a slow CI runner doesn't false-fail.
func TestInspect_RunsChecksInParallel(t *testing.T) {
	slow := func(_ context.Context, _ string) CheckResult {
		time.Sleep(200 * time.Millisecond)
		return CheckResult{Status: StatusOK}
	}
	checks := map[string]CheckFunc{"a": slow, "b": slow, "c": slow}

	start := time.Now()
	Inspect(context.Background(), "x", checks)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("checks did not run in parallel: elapsed=%s", elapsed)
	}
}

// summarize is the load-bearing scoring logic. Test it in isolation so a
// future tuning of the weights is a deliberate decision, not an accident.
func TestSummarize(t *testing.T) {
	cases := []struct {
		name        string
		results     []CheckResult
		wantVerdict Verdict
	}{
		{
			name:        "all unknown -> unknown verdict",
			results:     []CheckResult{{Status: StatusOK, Verdict: VerdictUnknown}},
			wantVerdict: VerdictUnknown,
		},
		{
			// One malicious alone is intentionally not enough — a single
			// jittery upstream shouldn't escalate to "malicious". It lands
			// at "suspicious" instead.
			name: "single malicious does not auto-escalate",
			results: []CheckResult{
				{Status: StatusOK, Verdict: VerdictMalicious},
				{Status: StatusOK, Verdict: VerdictClean},
			},
			wantVerdict: VerdictSuspicious,
		},
		{
			name: "two malicious votes -> malicious",
			results: []CheckResult{
				{Status: StatusOK, Verdict: VerdictMalicious},
				{Status: StatusOK, Verdict: VerdictMalicious},
			},
			wantVerdict: VerdictMalicious,
		},
		{
			name: "two suspicious -> suspicious",
			results: []CheckResult{
				{Status: StatusOK, Verdict: VerdictSuspicious},
				{Status: StatusOK, Verdict: VerdictSuspicious},
			},
			wantVerdict: VerdictSuspicious,
		},
		{
			name: "only clean -> clean",
			results: []CheckResult{
				{Status: StatusOK, Verdict: VerdictClean},
				{Status: StatusOK, Verdict: VerdictClean},
			},
			wantVerdict: VerdictClean,
		},
		{
			// Non-OK checks must not influence the score, otherwise a flaky
			// upstream that returns "malicious" with status=timeout would
			// poison the verdict.
			name: "malicious verdict on non-OK check is ignored",
			results: []CheckResult{
				{Status: StatusTimeout, Verdict: VerdictMalicious},
				{Status: StatusError, Verdict: VerdictMalicious},
			},
			wantVerdict: VerdictUnknown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := summarize(tc.results)
			if got.Verdict != tc.wantVerdict {
				t.Errorf("verdict: got %q, want %q (score=%d)", got.Verdict, tc.wantVerdict, got.Score)
			}
			if got.Score < 0 || got.Score > 100 {
				t.Errorf("score must be clamped to [0,100], got %d", got.Score)
			}
		})
	}
}
