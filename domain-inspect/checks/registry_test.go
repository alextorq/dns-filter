package checks

import (
	"context"
	"testing"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

func TestDefault_UsesInjectedChecks(t *testing.T) {
	check := func(marker string) domain_inspect.CheckFunc {
		return func(context.Context, string) domain_inspect.CheckResult {
			return domain_inspect.CheckResult{
				Status:  domain_inspect.StatusOK,
				Details: map[string]any{"marker": marker},
			}
		}
	}

	catalog := Default(DefaultDeps{
		LocalStats:   check("local_stats"),
		DNSResolve:   check("dns_resolve"),
		RDAP:         check("rdap"),
		CrtSh:        check("crtsh"),
		VirusTotal:   check("virustotal"),
		URLScan:      check("urlscan"),
		SafeBrowsing: check("safe_browsing"),
	})
	for _, name := range []string{"local_stats", "dns_resolve", "rdap", "crtsh", "virustotal", "urlscan", "safe_browsing"} {
		got := catalog[name](context.Background(), "example.com")
		if got.Details["marker"] != name {
			t.Errorf("catalog[%q] did not use injected check: got marker %v", name, got.Details["marker"])
		}
	}
}

func TestDefault_RejectsMissingInjectedChecks(t *testing.T) {
	check := func(context.Context, string) domain_inspect.CheckResult { return domain_inspect.CheckResult{} }
	cases := []struct {
		name string
		deps DefaultDeps
	}{
		{name: "local stats", deps: DefaultDeps{DNSResolve: check, RDAP: check, CrtSh: check, URLScan: check, VirusTotal: check, SafeBrowsing: check}},
		{name: "DNS resolve", deps: DefaultDeps{LocalStats: check, RDAP: check, CrtSh: check, URLScan: check, VirusTotal: check, SafeBrowsing: check}},
		{name: "RDAP", deps: DefaultDeps{LocalStats: check, DNSResolve: check, CrtSh: check, URLScan: check, VirusTotal: check, SafeBrowsing: check}},
		{name: "crt.sh", deps: DefaultDeps{LocalStats: check, DNSResolve: check, RDAP: check, URLScan: check, VirusTotal: check, SafeBrowsing: check}},
		{name: "urlscan", deps: DefaultDeps{LocalStats: check, DNSResolve: check, RDAP: check, CrtSh: check, VirusTotal: check, SafeBrowsing: check}},
		{name: "VirusTotal", deps: DefaultDeps{LocalStats: check, DNSResolve: check, RDAP: check, CrtSh: check, URLScan: check, SafeBrowsing: check}},
		{name: "Safe Browsing", deps: DefaultDeps{LocalStats: check, DNSResolve: check, RDAP: check, CrtSh: check, URLScan: check, VirusTotal: check}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected missing injected check to panic")
				}
			}()
			Default(tc.deps)
		})
	}
}
