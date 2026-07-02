package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
	"github.com/gin-gonic/gin"
)

type recordingLogger struct {
	calls int
}

func (l *recordingLogger) Info(...any) { l.calls++ }

func newTestHandlers() (*Handlers, *recordingLogger) {
	log := &recordingLogger{}
	return NewHandlers(
		func() map[string]domain_inspect.CheckFunc {
			return map[string]domain_inspect.CheckFunc{
				"stub": func(_ context.Context, _ string) domain_inspect.CheckResult {
					return domain_inspect.CheckResult{Status: domain_inspect.StatusOK, Verdict: domain_inspect.VerdictClean}
				},
			}
		},
		log,
	), log
}

func TestNewHandlers_RejectsMissingDependencies(t *testing.T) {
	log := &recordingLogger{}
	cases := []struct {
		name   string
		checks CheckFactory
		log    Logger
	}{
		{name: "missing checks", log: log},
		{name: "missing logger", checks: func() map[string]domain_inspect.CheckFunc { return nil }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected constructor to panic on incomplete wiring")
				}
			}()
			NewHandlers(tc.checks, tc.log)
		})
	}
}

func TestHandlers_RegisterRoutes_RejectsZeroValue(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected zero-value handlers to fail during route registration")
		}
	}()
	(&Handlers{}).RegisterRoutes(gin.New().Group("/api"))
}

func callInspect(t *testing.T, h *Handlers, query string) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.GET("/api/domain/inspect", h.Inspect)
	req := httptest.NewRequest(http.MethodGet, "/api/domain/inspect"+query, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestInspect_MissingDomain_Returns400(t *testing.T) {
	h, log := newTestHandlers()
	w := callInspect(t, h, "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if log.calls != 0 {
		t.Errorf("invalid requests must not be logged as completed inspections; got %d calls", log.calls)
	}
}

// A URL was passed instead of a bare hostname — we reject early rather than
// silently feeding "https://example.com/path" to RDAP / VT and getting back
// nonsense results.
func TestInspect_URLInsteadOfDomain_Returns400(t *testing.T) {
	h, _ := newTestHandlers()
	w := callInspect(t, h, "?domain=https://example.com/path")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for URL-shaped input, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// Wire-shape test: the handler must come back 200 with the expected envelope.
// Checks are stubbed so this stays hermetic and fast.
func TestInspect_ReturnsAggregatedShape(t *testing.T) {
	h, log := newTestHandlers()
	w := callInspect(t, h, "?domain=example.com")

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
	if log.calls != 1 {
		t.Errorf("successful inspection must be logged once; got %d calls", log.calls)
	}
}

// Locks in the lowercase normalization: input "Example.COM" must echo back as
// "example.com" so callers can rely on case-insensitive lookups.
func TestInspect_NormalizesDomainCase(t *testing.T) {
	h, _ := newTestHandlers()
	w := callInspect(t, h, "?domain=Example.COM")
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
