package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alextorq/dns-filter/config"
	"github.com/gin-gonic/gin"
)

// A present-but-invalid body is a client error, not the silent default. This is
// the #3 regression: a wrong-typed filter_docker must return 400, not 200 with
// the opposite filtering. (Runs only in LAN mode, the default, so it never
// reaches the real network scan — the bind fails first.)
func TestDiscoverHandler_MalformedBody400(t *testing.T) {
	if config.GetConfig().Mode != config.ModeLAN {
		t.Skip("discover handler only reaches body-binding in LAN mode")
	}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/clients/discover",
		strings.NewReader(`{"filter_docker": "not-a-bool"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	Discover(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on malformed body, got %d (%s)", w.Code, w.Body.String())
	}
}
