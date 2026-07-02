package checks

import (
	"context"
	"testing"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

func TestDefault_UsesInjectedLocalStats(t *testing.T) {
	local := func(context.Context, string) domain_inspect.CheckResult {
		return domain_inspect.CheckResult{Status: domain_inspect.StatusOK, Verdict: domain_inspect.VerdictClean}
	}

	catalog := Default(local)
	got := catalog["local_stats"](context.Background(), "example.com")
	if got.Verdict != domain_inspect.VerdictClean {
		t.Errorf("catalog did not use injected local_stats check: got %q", got.Verdict)
	}
}

func TestDefault_RejectsMissingLocalStats(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected missing local_stats check to panic")
		}
	}()
	Default(nil)
}
