// Package inspect is the reputation-enrichment layer that sits between the
// lexical candidate queue (inspect/db) and the manual domain-inspect checks.
// The Adapter runs a reduced, cache-aware subset of those checks for one FQDN
// and collapses the result into a verdict plus the reasons that explain it.
package inspect

import (
	"context"
	"errors"
	"fmt"
	"time"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
	"github.com/alextorq/dns-filter/domain-inspect/checks"
	collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	inspect_db "github.com/alextorq/dns-filter/suggest-to-block/inspect/db"
	"golang.org/x/net/publicsuffix"
)

// ErrRateLimited signals that a provider returned HTTP 429 during the
// inspection. The worker treats it as "stop the run and back off", not as a
// per-domain failure: the quota is exhausted for every remaining domain too.
var ErrRateLimited = errors.New("inspect: provider rate-limited")

// Result is the adapter's collapsed view of an inspection. Verdict is the
// domain-inspect summary (clean|suspicious|malicious|unknown). Reasons carry
// the inspect_* codes that explain it — recorded even when the summary is
// "unknown" (e.g. a young domain on its own scores below the summary
// threshold) so the worker can still see the individual signals.
type Result struct {
	Verdict string
	Reasons []collect.Reason
}

// Adapter runs the reduced, cache-aware reputation check set for one FQDN and
// collapses the result into a verdict plus the reasons that explain it.
type Adapter struct {
	repo *inspect_db.Repo
	// rdapTTL bounds how long a cached registration age stays fresh. Age grows
	// only monotonically, so a generous TTL is safe; wired from config.
	rdapTTL time.Duration
	checks  map[string]domain_inspect.CheckFunc
}

// NewAdapter builds the production adapter: RDAP (cache-wrapped), VirusTotal and
// Safe Browsing only. crt.sh / urlscan / dns_resolve / local_stats are
// deliberately excluded — for an already-allowed candidate they return
// "unknown" and add nothing but latency and quota pressure.
func NewAdapter(repo *inspect_db.Repo, rdapTTL time.Duration) *Adapter {
	a := &Adapter{repo: repo, rdapTTL: rdapTTL}
	a.checks = map[string]domain_inspect.CheckFunc{
		"rdap":          a.withRDAPCache(checks.RDAPAge),
		"virustotal":    checks.VirusTotal,
		"safe_browsing": checks.SafeBrowsing,
	}
	return a
}

// Inspect runs the reduced check set against fqdn and collapses the outcome.
// Any rate-limited check short-circuits to ErrRateLimited so the worker pauses
// before a complete verdict is even assembled — a domain delayed one cycle is
// fine; hammering an exhausted quota is not.
func (a *Adapter) Inspect(ctx context.Context, fqdn string) (Result, error) {
	res := domain_inspect.Inspect(ctx, fqdn, a.checks)
	for _, c := range res.Checks {
		if c.Status == domain_inspect.StatusRateLimited {
			return Result{}, ErrRateLimited
		}
	}
	return Result{
		Verdict: string(res.Summary.Verdict),
		Reasons: mapReasons(res),
	}, nil
}

// mapReasons translates the per-check results into inspect_* reason codes. Each
// signal is recorded independently of the aggregate summary so the worker (and
// the UI) can see exactly which provider flagged the domain.
func mapReasons(res domain_inspect.InspectResult) []collect.Reason {
	var reasons []collect.Reason
	for _, c := range res.Checks {
		if c.Status != domain_inspect.StatusOK {
			continue
		}
		switch c.Name {
		case "virustotal":
			if c.Verdict == domain_inspect.VerdictMalicious {
				n, _ := c.Details["malicious"].(int)
				reasons = append(reasons, collect.Reason{
					Code:  collect.CodeInspectVTMalicious,
					Match: fmt.Sprintf("malicious=%d", n),
				})
			}
		case "safe_browsing":
			if c.Verdict == domain_inspect.VerdictMalicious {
				reasons = append(reasons, collect.Reason{Code: collect.CodeInspectSafeBrowsing})
			}
		case "rdap":
			if c.Verdict == domain_inspect.VerdictSuspicious {
				age, _ := c.Details["age_days"].(int)
				reasons = append(reasons, collect.Reason{
					Code:  collect.CodeInspectRDAPYoung,
					Match: fmt.Sprintf("age_days=%d", age),
				})
			}
		}
	}
	// A clean summary is a real endorsement (e.g. Safe Browsing empty match on a
	// well-aged domain) — record it so the worker can drop/deactivate the
	// candidate instead of leaving it pending.
	if res.Summary.Verdict == domain_inspect.VerdictClean {
		reasons = append(reasons, collect.Reason{Code: collect.CodeInspectCleanEndorsed})
	}
	return reasons
}

// withRDAPCache wraps an RDAP check func with the registrable-keyed cache so
// sibling FQDNs under one eTLD+1 (a.evil.com, b.evil.com) do not each re-query
// RDAP. VirusTotal / Safe Browsing answer per-FQDN and are cached on the
// candidate row instead; RDAP is the only per-registrable check, hence the
// dedicated cache. A non-registrable input is delegated to inner so it returns
// the honest skip/unknown rather than poisoning the cache.
func (a *Adapter) withRDAPCache(inner domain_inspect.CheckFunc) domain_inspect.CheckFunc {
	return func(ctx context.Context, fqdn string) domain_inspect.CheckResult {
		reg, err := publicsuffix.EffectiveTLDPlusOne(fqdn)
		if err != nil {
			return inner(ctx, fqdn)
		}
		if c, ok, _ := a.repo.GetRDAP(reg, a.rdapTTL); ok {
			inspectRDAPCacheHits.Inc()
			return domain_inspect.CheckResult{
				Status:  domain_inspect.StatusOK,
				Verdict: checks.RDAPVerdictForAge(c.AgeDays),
				Details: map[string]any{"age_days": c.AgeDays, "queried_domain": reg, "cached": true},
			}
		}
		res := inner(ctx, fqdn)
		if res.Status == domain_inspect.StatusOK {
			if age, ok := res.Details["age_days"].(int); ok {
				_ = a.repo.PutRDAP(reg, age)
			}
		}
		return res
	}
}
