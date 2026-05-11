// Package domain_inspect runs a fan-out of independent reputation/diagnostic
// checks against a single domain and aggregates the verdicts. Each check is
// isolated: a failure or timeout in one never affects the others, and the
// caller always receives a full list of per-check results plus a summary.
//
// The package is intentionally storage-less — no caching yet, every call hits
// the underlying sources. Caching will be added later as a separate layer.
package domain_inspect

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type CheckStatus string

const (
	StatusOK      CheckStatus = "ok"
	StatusError   CheckStatus = "error"
	StatusSkipped CheckStatus = "skipped"
	StatusTimeout CheckStatus = "timeout"
)

type Verdict string

const (
	VerdictUnknown    Verdict = "unknown"
	VerdictClean      Verdict = "clean"
	VerdictSuspicious Verdict = "suspicious"
	VerdictMalicious  Verdict = "malicious"
)

type CheckResult struct {
	Name       string         `json:"name"`
	Status     CheckStatus    `json:"status"`
	Verdict    Verdict        `json:"verdict,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
	DurationMs int64          `json:"duration_ms"`
	Error      string         `json:"error,omitempty"`
}

type CheckFunc func(ctx context.Context, domain string) CheckResult

type Summary struct {
	Verdict Verdict `json:"verdict"`
	Score   int     `json:"score"`
}

type InspectResult struct {
	Domain  string        `json:"domain"`
	Checks  []CheckResult `json:"checks"`
	Summary Summary       `json:"summary"`
}

// Inspect runs every check from `checks` in parallel against `domain`, bounded
// by the provided context. Per-check panics are recovered into an error
// result so one bad check cannot crash the request.
func Inspect(ctx context.Context, domain string, checks map[string]CheckFunc) InspectResult {
	results := make([]CheckResult, 0, len(checks))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for name, fn := range checks {
		wg.Add(1)
		go func(name string, fn CheckFunc) {
			defer wg.Done()
			start := time.Now()
			res := runSafe(ctx, name, fn, domain)
			res.Name = name
			res.DurationMs = time.Since(start).Milliseconds()
			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}(name, fn)
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })

	return InspectResult{
		Domain:  domain,
		Checks:  results,
		Summary: summarize(results),
	}
}

func runSafe(ctx context.Context, name string, fn CheckFunc, domain string) (res CheckResult) {
	defer func() {
		if r := recover(); r != nil {
			res = CheckResult{Name: name, Status: StatusError, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()
	return fn(ctx, domain)
}

// summarize collapses per-check verdicts into a single recommendation.
// Weights are deliberate: a single "malicious" from a high-signal source is
// almost always enough on its own; suspicious results stack to that threshold.
func summarize(results []CheckResult) Summary {
	score := 0
	for _, r := range results {
		if r.Status != StatusOK {
			continue
		}
		switch r.Verdict {
		case VerdictMalicious:
			score += 40
		case VerdictSuspicious:
			score += 15
		case VerdictClean:
			score -= 5
		}
	}

	verdict := VerdictUnknown
	switch {
	case score >= 50:
		verdict = VerdictMalicious
	case score >= 20:
		verdict = VerdictSuspicious
	case score < 0:
		verdict = VerdictClean
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return Summary{Verdict: verdict, Score: score}
}
