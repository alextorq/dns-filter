package checks

import (
	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

type DefaultDeps struct {
	LocalStats   domain_inspect.CheckFunc
	DNSResolve   domain_inspect.CheckFunc
	RDAP         domain_inspect.CheckFunc
	CrtSh        domain_inspect.CheckFunc
	URLScan      domain_inspect.CheckFunc
	VirusTotal   domain_inspect.CheckFunc
	SafeBrowsing domain_inspect.CheckFunc
}

// Default returns the full catalog of inspection checks. The map shape lets
// callers easily drop checks (for example by name) or replace one in tests.
// New checks should be added here so the HTTP endpoint picks them up for free.
func Default(deps DefaultDeps) map[string]domain_inspect.CheckFunc {
	if deps.LocalStats == nil {
		panic("domain-inspect/checks: local stats check is required")
	}
	if deps.DNSResolve == nil {
		panic("domain-inspect/checks: DNS resolve check is required")
	}
	if deps.RDAP == nil {
		panic("domain-inspect/checks: RDAP check is required")
	}
	if deps.CrtSh == nil {
		panic("domain-inspect/checks: crt.sh check is required")
	}
	if deps.URLScan == nil {
		panic("domain-inspect/checks: urlscan check is required")
	}
	if deps.VirusTotal == nil {
		panic("domain-inspect/checks: VirusTotal check is required")
	}
	if deps.SafeBrowsing == nil {
		panic("domain-inspect/checks: Safe Browsing check is required")
	}
	return map[string]domain_inspect.CheckFunc{
		"local_stats":   deps.LocalStats,
		"dns_resolve":   deps.DNSResolve,
		"rdap":          deps.RDAP,
		"crtsh":         deps.CrtSh,
		"virustotal":    deps.VirusTotal,
		"urlscan":       deps.URLScan,
		"safe_browsing": deps.SafeBrowsing,
	}
}
