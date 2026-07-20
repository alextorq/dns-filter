package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alextorq/dns-filter/filter"
	runtime_state "github.com/alextorq/dns-filter/filter/runtime-state"

	"github.com/gin-gonic/gin"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

// stubRepo / stubBloom / stubCache satisfy the narrow filter.* ports without
// touching a DB. Pause / Resume / Status handlers don't read from any of them
// — they only flip atomic config fields — so stubs return zero values.
type stubRepo struct{}

func (stubRepo) GetAllActiveURLs() ([]string, error)           { return nil, nil }
func (stubRepo) IsActivelyBlocked(domain string) (bool, error) { return false, nil }

type stubBloom struct{}

func (stubBloom) DomainExist(domain string) bool { return false }
func (stubBloom) UpdateFilter(rows []string)     {}

type stubCache struct{}

func (stubCache) Get(key string) (bool, bool) { return false, false }
func (stubCache) Add(key string, val bool)    {}
func (stubCache) Clear()                      {}

type stubLog struct{}

func (stubLog) Info(args ...any)  {}
func (stubLog) Debug(args ...any) {}
func (stubLog) Error(err error)   {}

func newTestHandlers() (*Handlers, *runtime_state.State) {
	state := runtime_state.New(true)
	module := filter.NewModule(stubRepo{}, stubBloom{}, stubCache{}, state, fixedClock{now: time.Now()}, stubLog{})
	return &Handlers{Module: module}, state
}

func postJSON(t *testing.T, fn gin.HandlerFunc, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST(path, fn)
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// PauseFilter must map ErrInvalidDuration to 400, not the default 500. The
// handler's switch is the only place that knows the mapping; without this
// test, a refactor that loses the case silently downgrades validation
// errors to "internal" and the UI shows a misleading message.
func TestHandlerPauseFilter_InvalidDuration_Returns400(t *testing.T) {
	h, _ := newTestHandlers()
	w := postJSON(t, h.PauseFilter, "/api/filter/pause", PauseFilterRequest{Minutes: 7})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// PauseFilter must map ErrFilterDisabled to 409 (Conflict) — the resource
// state precludes the operation. 500 here would mask a known business error
// behind a server fault.
func TestHandlerPauseFilter_FilterDisabled_Returns409(t *testing.T) {
	h, state := newTestHandlers()
	state.SetEnabled(false)

	w := postJSON(t, h.PauseFilter, "/api/filter/pause", PauseFilterRequest{Minutes: 5})
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestHandlerPauseFilter_BadJSON_Returns400(t *testing.T) {
	h, _ := newTestHandlers()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/filter/pause", h.PauseFilter)
	req := httptest.NewRequest(http.MethodPost, "/api/filter/pause", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// Happy path: PauseFilter returns 200 and a future deadline. Pins both the
// success status and the response payload shape so a refactor that drops
// PausedUntil from the body would fail here.
func TestHandlerPauseFilter_Valid_Returns200WithDeadline(t *testing.T) {
	h, _ := newTestHandlers()
	w := postJSON(t, h.PauseFilter, "/api/filter/pause", PauseFilterRequest{Minutes: 5})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var resp FilterStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.PausedUntil <= 0 {
		t.Errorf("expected positive PausedUntil, got %d", resp.PausedUntil)
	}
	if !resp.Status {
		t.Errorf("expected Status=true (filter still enabled), got false")
	}
}

// ResumeFilter must always return 200 with PausedUntil=0, even when there
// was no active pause to clear.
func TestHandlerResumeFilter_NoActivePause_Returns200(t *testing.T) {
	h, _ := newTestHandlers()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/filter/resume", h.ResumeFilter)
	req := httptest.NewRequest(http.MethodPost, "/api/filter/resume", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var resp FilterStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.PausedUntil != 0 {
		t.Errorf("expected PausedUntil=0 after resume, got %d", resp.PausedUntil)
	}
}

// ChangeFilterStatus toggles Enabled and clears any active pause atomically.
func TestHandlerChangeFilterStatus_TogglesAndClearsPause(t *testing.T) {
	h, state := newTestHandlers()
	// Pre-arm a pause so we can assert it gets cleared by the toggle.
	if _, err := h.Module.Pause(5); err != nil {
		t.Fatalf("Pause failed: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/filter/change-status", h.ChangeFilterStatus)
	req := httptest.NewRequest(http.MethodPost, "/api/filter/change-status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if state.Enabled() {
		t.Error("Enabled must flip from true to false on toggle")
	}
	if state.PausedUntil() != 0 {
		t.Error("toggle must clear active pause")
	}
}
