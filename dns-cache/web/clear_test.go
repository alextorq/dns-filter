package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// fakeFlusher records how many times Clear was called and what it returned so
// the test can both assert behaviour and prove the handler used the injected
// dependency (and not the real package-global cache).
type fakeFlusher struct {
	cleared    int
	clearCalls int
}

func (f *fakeFlusher) Clear() int {
	f.clearCalls++
	return f.cleared
}

func withFlusher(t *testing.T, f cacheFlusher) {
	t.Helper()
	prev := flusherFactory
	flusherFactory = func() cacheFlusher { return f }
	t.Cleanup(func() { flusherFactory = prev })
}

func callClear(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/dns-cache/clear", ClearCache)
	req := httptest.NewRequest(http.MethodPost, "/api/dns-cache/clear", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Happy path: 200 + reported count matches what the cache returned, and the
// underlying Clear was called exactly once. The "exactly once" check protects
// against a future refactor that double-flushes (e.g. via a retry wrapper)
// and silently makes the operation non-idempotent in dashboards.
func TestClearCache_PopulatedCache_Returns200AndCount(t *testing.T) {
	f := &fakeFlusher{cleared: 42}
	withFlusher(t, f)

	w := callClear(t)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if f.clearCalls != 1 {
		t.Fatalf("expected Clear to be called exactly once, got %d", f.clearCalls)
	}

	var resp ClearCacheResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v (body=%s)", err, w.Body.String())
	}
	if resp.Cleared != 42 {
		t.Errorf("expected cleared=42, got %d", resp.Cleared)
	}
}

// Edge case: operator triggers a flush on an already-cold cache. The handler
// must still 200 with cleared=0 — the UI uses that to render "cache was
// already empty" instead of an error toast.
func TestClearCache_EmptyCache_Returns200AndZero(t *testing.T) {
	f := &fakeFlusher{cleared: 0}
	withFlusher(t, f)

	w := callClear(t)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on empty cache, got %d", w.Code)
	}
	var resp ClearCacheResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp.Cleared != 0 {
		t.Errorf("expected cleared=0, got %+v", resp)
	}
}

// Method-shape contract: a flush is a state mutation, so the route must be
// POST only. CSRF defence proper lives in the cookie's SameSite policy and
// CORS — this test just pins the verb so an accidental r.Any / r.GET in a
// future refactor can't silently make the endpoint reachable via a method
// it shouldn't be.
func TestClearCache_RejectsNonPOST(t *testing.T) {
	f := &fakeFlusher{}
	withFlusher(t, f)

	r := gin.New()
	r.POST("/api/dns-cache/clear", ClearCache)
	req := httptest.NewRequest(http.MethodGet, "/api/dns-cache/clear", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 404/405 for GET, got %d", w.Code)
	}
	if f.clearCalls != 0 {
		t.Fatalf("Clear must not run on non-POST, got %d calls", f.clearCalls)
	}
}
