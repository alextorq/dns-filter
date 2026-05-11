package checks

import (
	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

// Default returns the full catalog of inspection checks. The map shape lets
// callers easily drop checks (for example by name) or replace one in tests.
// New checks should be added here so the HTTP endpoint picks them up for free.
func Default() map[string]domain_inspect.CheckFunc {
	return map[string]domain_inspect.CheckFunc{
		"local_stats": LocalStats,
		"dns_resolve": DNSResolve,
		"rdap":        RDAPAge,
		"crtsh":       CrtSh,
		"virustotal":     VirusTotal,
		"urlscan":        URLScan,
		"safe_browsing":  SafeBrowsing,
	}
}
