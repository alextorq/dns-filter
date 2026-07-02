package checks

import (
	"context"
	"testing"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

func TestDefault_UsesInjectedChecks(t *testing.T) {
	local := func(context.Context, string) domain_inspect.CheckResult {
		return domain_inspect.CheckResult{Status: domain_inspect.StatusOK, Verdict: domain_inspect.VerdictClean}
	}
	urlScan := func(context.Context, string) domain_inspect.CheckResult {
		return domain_inspect.CheckResult{Status: domain_inspect.StatusOK, Verdict: domain_inspect.VerdictSuspicious}
	}

	catalog := Default(DefaultDeps{LocalStats: local, URLScan: urlScan})
	if got := catalog["local_stats"](context.Background(), "example.com"); got.Verdict != domain_inspect.VerdictClean {
		t.Errorf("catalog did not use injected local_stats check: got %q", got.Verdict)
	}
	if got := catalog["urlscan"](context.Background(), "example.com"); got.Verdict != domain_inspect.VerdictSuspicious {
		t.Errorf("catalog did not use injected urlscan check: got %q", got.Verdict)
	}
}

func TestDefault_RejectsMissingInjectedChecks(t *testing.T) {
	check := func(context.Context, string) domain_inspect.CheckResult { return domain_inspect.CheckResult{} }
	cases := []struct {
		name string
		deps DefaultDeps
	}{
		{name: "local stats", deps: DefaultDeps{URLScan: check}},
		{name: "urlscan", deps: DefaultDeps{LocalStats: check}},
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
