package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	app_db "github.com/alextorq/dns-filter/db"
	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
	"github.com/gin-gonic/gin"
)

// stubChecks replaces the real catalog with deterministic in-memory ones so
// the handler tests don't reach the network. Restored by t.Cleanup so a
// failing test cannot leak the stub into siblings.
func stubChecks(t *testing.T) {
	t.Helper()
	prev := checksFactory
	checksFactory = func() map[string]domain_inspect.CheckFunc {
		return map[string]domain_inspect.CheckFunc{
			"stub": func(_ context.Context, _ string) domain_inspect.CheckResult {
				return domain_inspect.CheckResult{Status: domain_inspect.StatusOK, Verdict: domain_inspect.VerdictClean}
			},
		}
	}
	t.Cleanup(func() { checksFactory = prev })
}

func TestMain(m *testing.M) {
	// The handler reaches into the DB (via local_stats), so we need an
	// isolated SQLite file. Chdir to a tmp dir to redirect ./filter.sqlite.
	tmp, err := os.MkdirTemp("", "domain-inspect-web-test-*")
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(tmp); err != nil {
		os.RemoveAll(tmp)
		panic(err)
	}
	// Touch the connection so migrations run (the handler depends on the
	// block/allow tables existing).
	_ = app_db.GetConnection()
	gin.SetMode(gin.TestMode)

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func callInspect(t *testing.T, query string) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.GET("/api/domain/inspect", Inspect)
	req := httptest.NewRequest(http.MethodGet, "/api/domain/inspect"+query, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestInspect_MissingDomain_Returns400(t *testing.T) {
	w := callInspect(t, "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// A URL was passed instead of a bare hostname — we reject early rather than
// silently feeding "https://example.com/path" to RDAP / VT and getting back
// nonsense results.
func TestInspect_URLInsteadOfDomain_Returns400(t *testing.T) {
	w := callInspect(t, "?domain=https://example.com/path")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for URL-shaped input, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// Wire-shape test: the handler must come back 200 with the expected envelope.
// Checks are stubbed so this stays hermetic and fast.
func TestInspect_ReturnsAggregatedShape(t *testing.T) {
	stubChecks(t)

	w := callInspect(t, "?domain=example.com")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}

	var got domain_inspect.InspectResult
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Domain != "example.com" {
		t.Errorf("domain echo: got %q, want example.com", got.Domain)
	}
	if len(got.Checks) == 0 {
		t.Error("expected at least one check result")
	}
	// Every check result must carry a name — that's the field the UI keys on.
	for i, c := range got.Checks {
		if c.Name == "" {
			t.Errorf("check %d has empty name", i)
		}
	}
}

// Locks in the lowercase normalization: input "Example.COM" must echo back as
// "example.com" so callers can rely on case-insensitive lookups.
func TestInspect_NormalizesDomainCase(t *testing.T) {
	stubChecks(t)

	w := callInspect(t, "?domain=Example.COM")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got domain_inspect.InspectResult
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Domain != "example.com" {
		t.Errorf("domain not lowercased: got %q", got.Domain)
	}
}
