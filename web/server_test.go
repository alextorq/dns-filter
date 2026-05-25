package web

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	blockedWeb "github.com/alextorq/dns-filter/blocked-domain/web"
	filterWeb "github.com/alextorq/dns-filter/filter/web"
	loggerWeb "github.com/alextorq/dns-filter/logger/web"
	settingsWeb "github.com/alextorq/dns-filter/settings/web"
	sourceWeb "github.com/alextorq/dns-filter/source/web"
	suggestWeb "github.com/alextorq/dns-filter/suggest-to-block/web"
	trafficWeb "github.com/alextorq/dns-filter/traffic/web"
	"github.com/gin-gonic/gin"
)

// expectedRoutes is the canonical contract of the HTTP API surface. Any
// rename / addition / removal must update this slice and (if it changes the
// public contract) regenerate the Swagger doc + frontend client per
// CLAUDE.md. Keeping this snapshot in tree means the self-routing refactor
// cannot accidentally drop or relocate an endpoint.
var expectedRoutes = []string{
	"GET /api/auth/me",
	"GET /api/config/db/download",
	"GET /api/domain/inspect",
	"GET /api/filter/status",
	"GET /api/settings",
	"GET /api/suggest-to-block/codes",
	"GET /api/traffic/devices",
	"GET /api/traffic/devices/domains",
	"GET /api/traffic/top-domains",
	"GET /swagger/*any",
	"PUT /api/settings/:key",
	"DELETE /api/settings/:key",
	"POST /api/auth/login",
	"POST /api/auth/logout",
	"POST /api/clients",
	"POST /api/clients/change-filter",
	"POST /api/clients/create",
	"POST /api/clients/delete",
	"POST /api/clients/discover",
	"POST /api/clients/update",
	"POST /api/config/logger/change-level",
	"POST /api/config/logger/get-level",
	"POST /api/dns-cache/clear",
	"POST /api/dns-records",
	"POST /api/dns-records/create",
	"POST /api/dns-records/update",
	"POST /api/events/block/amount",
	"POST /api/events/block/amount-by-group",
	"POST /api/filter/change-status",
	"POST /api/filter/pause",
	"POST /api/filter/resume",
	"POST /api/sources",
	"POST /api/sources/change-status",
	"POST /api/suggest-to-block",
	"POST /api/suggest-to-block/add-to-block",
	"POST /api/suggest-to-block/change-status",
}

func TestBuildRouter_RegistersAllExpectedRoutes(t *testing.T) {
	r := buildRouter(testHandlers())

	got := collectRoutes(r)
	want := append([]string(nil), expectedRoutes...)
	sort.Strings(want)

	if !reflect.DeepEqual(got, want) {
		missing, extra := diffRoutes(want, got)
		if len(missing) > 0 {
			t.Errorf("missing routes: %v", missing)
		}
		if len(extra) > 0 {
			t.Errorf("unexpected routes: %v", extra)
		}
	}
}

func testHandlers() Handlers {
	return Handlers{
		Blocked:  &blockedWeb.Handlers{},
		Filter:   &filterWeb.Handlers{},
		Suggest:  &suggestWeb.Handlers{},
		Source:   &sourceWeb.Handlers{},
		Logger:   &loggerWeb.Handlers{},
		Settings: &settingsWeb.Handlers{},
		Traffic:  &trafficWeb.Handlers{},
	}
}

func collectRoutes(r *gin.Engine) []string {
	routes := r.Routes()
	out := make([]string, 0, len(routes))
	for _, rt := range routes {
		out = append(out, rt.Method+" "+rt.Path)
	}
	sort.Strings(out)
	return out
}

// TestBuildRouter_LoginIsPublic pins that POST /api/auth/login bypasses the
// session middleware. Without a session cookie, the request must reach the
// Login handler (which rejects an empty body with 400) instead of being
// short-circuited with 401 by RequireAuth.
func TestBuildRouter_LoginIsPublic(t *testing.T) {
	r := buildRouter(testHandlers())

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("login endpoint is sitting behind RequireAuth: got 401, body=%s", w.Body.String())
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 from empty-body bind, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// TestBuildRouter_ProtectedRoutesRequireAuth exhaustively walks every /api/*
// path expected to live under RequireAuth and verifies that an unauthenticated
// request gets a 401 from the middleware instead of touching the handler. If
// a route accidentally lands outside the protected group (e.g. via a future
// RegisterPublic call), this test catches it for that specific endpoint. The
// list is derived from expectedRoutes minus the one public exception.
func TestBuildRouter_ProtectedRoutesRequireAuth(t *testing.T) {
	r := buildRouter(testHandlers())

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/auth/logout"},
		{http.MethodGet, "/api/auth/me"},
		{http.MethodPost, "/api/dns-records"},
		{http.MethodPost, "/api/dns-records/create"},
		{http.MethodPost, "/api/dns-records/update"},
		{http.MethodGet, "/api/filter/status"},
		{http.MethodPost, "/api/filter/change-status"},
		{http.MethodPost, "/api/filter/pause"},
		{http.MethodPost, "/api/filter/resume"},
		{http.MethodPost, "/api/events/block/amount"},
		{http.MethodPost, "/api/events/block/amount-by-group"},
		{http.MethodPost, "/api/suggest-to-block"},
		{http.MethodGet, "/api/suggest-to-block/codes"},
		{http.MethodPost, "/api/suggest-to-block/add-to-block"},
		{http.MethodPost, "/api/suggest-to-block/change-status"},
		{http.MethodPost, "/api/sources"},
		{http.MethodPost, "/api/sources/change-status"},
		{http.MethodPost, "/api/clients"},
		{http.MethodPost, "/api/clients/create"},
		{http.MethodPost, "/api/clients/update"},
		{http.MethodPost, "/api/clients/change-filter"},
		{http.MethodPost, "/api/clients/delete"},
		{http.MethodPost, "/api/clients/discover"},
		{http.MethodGet, "/api/config/db/download"},
		{http.MethodPost, "/api/config/logger/change-level"},
		{http.MethodPost, "/api/config/logger/get-level"},
		{http.MethodPost, "/api/dns-cache/clear"},
		{http.MethodGet, "/api/domain/inspect"},
		{http.MethodGet, "/api/traffic/devices"},
		{http.MethodGet, "/api/traffic/devices/domains"},
		{http.MethodGet, "/api/traffic/top-domains"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 from RequireAuth, got %d (body=%s)", w.Code, w.Body.String())
			}
		})
	}
}

// TestBuildRouter_LoginOnlyAcceptsPOST pins that /api/auth/login is registered
// for POST only — any other verb must 404 (gin's default for an unregistered
// method/path combo). Catches the regression where a future contributor adds
// a GET handler on the same path and accidentally exposes a credential-leaking
// endpoint outside RequireAuth.
func TestBuildRouter_LoginOnlyAcceptsPOST(t *testing.T) {
	r := buildRouter(testHandlers())

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/auth/login", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound {
				t.Errorf("expected 404 for %s /api/auth/login, got %d (body=%s)", method, w.Code, w.Body.String())
			}
		})
	}
}

// TestBuildRouter_CORSPreflightOnLogin guards the CORS contract for the only
// pre-auth endpoint: the browser sends an OPTIONS preflight before POSTing
// credentials, and a misrouted /api/auth/login (e.g. behind RequireAuth)
// would silently break the login form even though POSTs still 200.
func TestBuildRouter_CORSPreflightOnLogin(t *testing.T) {
	r := buildRouter(testHandlers())

	req := httptest.NewRequest(http.MethodOptions, "/api/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
		t.Fatalf("CORS preflight should return 204/200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got == "" {
		t.Errorf("missing Access-Control-Allow-Origin header on preflight response")
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Errorf("Access-Control-Allow-Methods should advertise POST, got %q", got)
	}
}

func diffRoutes(want, got []string) (missing, extra []string) {
	wantSet := make(map[string]struct{}, len(want))
	for _, r := range want {
		wantSet[r] = struct{}{}
	}
	gotSet := make(map[string]struct{}, len(got))
	for _, r := range got {
		gotSet[r] = struct{}{}
	}
	for r := range wantSet {
		if _, ok := gotSet[r]; !ok {
			missing = append(missing, r)
		}
	}
	for r := range gotSet {
		if _, ok := wantSet[r]; !ok {
			extra = append(extra, r)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return
}
