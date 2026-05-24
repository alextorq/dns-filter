package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alextorq/dns-filter/settings"
	"github.com/gin-gonic/gin"
)

type fakeService struct {
	list       []settings.Effective
	listErr    error
	setErr     error
	resetErr   error
	setCalls   [][2]string
	resetCalls []string
}

func (f *fakeService) List() ([]settings.Effective, error) { return f.list, f.listErr }

func (f *fakeService) Set(key, raw string) error {
	f.setCalls = append(f.setCalls, [2]string{key, raw})
	return f.setErr
}

func (f *fakeService) Reset(key string) error {
	f.resetCalls = append(f.resetCalls, key)
	return f.resetErr
}

func newTestRouter(svc Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &Handlers{Service: svc}
	h.RegisterRoutes(r.Group("/api"))
	return r
}

func do(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestListSettings_OK(t *testing.T) {
	svc := &fakeService{list: []settings.Effective{{Key: "log_level", Value: "INFO"}}}
	r := newTestRouter(svc)

	w := do(r, http.MethodGet, "/api/settings", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "log_level") {
		t.Errorf("body should include the setting, got %s", w.Body.String())
	}
}

func TestListSettings_ServiceError(t *testing.T) {
	svc := &fakeService{listErr: errString("db down")}
	r := newTestRouter(svc)

	w := do(r, http.MethodGet, "/api/settings", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestUpdateSetting_OK(t *testing.T) {
	svc := &fakeService{}
	r := newTestRouter(svc)

	w := do(r, http.MethodPut, "/api/settings/log_level", `{"value":"DEBUG"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if len(svc.setCalls) != 1 || svc.setCalls[0] != [2]string{"log_level", "DEBUG"} {
		t.Errorf("expected Set(log_level, DEBUG), got %v", svc.setCalls)
	}
}

func TestUpdateSetting_BadBody(t *testing.T) {
	svc := &fakeService{}
	r := newTestRouter(svc)

	w := do(r, http.MethodPut, "/api/settings/log_level", `not json`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if len(svc.setCalls) != 0 {
		t.Error("bad body must not reach the service")
	}
}

func TestUpdateSetting_InvalidValueIs400(t *testing.T) {
	svc := &fakeService{setErr: settings.ErrInvalidValue}
	r := newTestRouter(svc)

	w := do(r, http.MethodPut, "/api/settings/doh_upstream", `{"value":"nope"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestUpdateSetting_UnknownKeyIs404(t *testing.T) {
	svc := &fakeService{setErr: settings.ErrUnknownKey}
	r := newTestRouter(svc)

	w := do(r, http.MethodPut, "/api/settings/whatever", `{"value":"x"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestUpdateSetting_PersistErrorIs500(t *testing.T) {
	svc := &fakeService{setErr: errString("disk full")}
	r := newTestRouter(svc)

	w := do(r, http.MethodPut, "/api/settings/log_level", `{"value":"INFO"}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestResetSetting_OK(t *testing.T) {
	svc := &fakeService{}
	r := newTestRouter(svc)

	w := do(r, http.MethodDelete, "/api/settings/log_level", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(svc.resetCalls) != 1 || svc.resetCalls[0] != "log_level" {
		t.Errorf("expected Reset(log_level), got %v", svc.resetCalls)
	}
}

func TestResetSetting_UnknownKeyIs404(t *testing.T) {
	svc := &fakeService{resetErr: settings.ErrUnknownKey}
	r := newTestRouter(svc)

	w := do(r, http.MethodDelete, "/api/settings/whatever", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// errString is a tiny error helper so tests can simulate generic failures
// without importing errors at every call site.
type errString string

func (e errString) Error() string { return string(e) }
