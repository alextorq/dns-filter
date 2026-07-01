package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alextorq/dns-filter/auth/business"
	authDb "github.com/alextorq/dns-filter/auth/db"
	"github.com/gin-gonic/gin"
)

type fakeService struct {
	user         *authDb.User
	session      *authDb.Session
	authErr      error
	resolveErr   error
	revokeErr    error
	resolvedWith string
	revoked      string
}

func (s *fakeService) Authenticate(string, string) (*authDb.User, *authDb.Session, error) {
	return s.user, s.session, s.authErr
}

func (s *fakeService) ResolveSession(token string) (*authDb.Session, *authDb.User, error) {
	s.resolvedWith = token
	return s.session, s.user, s.resolveErr
}

func (s *fakeService) RevokeSession(token string) error {
	s.revoked = token
	return s.revokeErr
}

func TestLogin_SetsConfiguredSecureCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeService{
		user:    &authDb.User{ID: 7, Login: "admin"},
		session: &authDb.Session{Token: "token", UserID: 7, ExpiresAt: time.Now().Add(time.Hour)},
	}
	h := &Handlers{Service: service, CookieSecure: true, CookieSameSite: "None"}
	r := gin.New()
	r.POST("/login", h.Login)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"login":"admin","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != SessionCookieName || cookie.Value != "token" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteNoneMode {
		t.Fatalf("unexpected session cookie: %+v", cookie)
	}
}

func TestLogin_RejectsInvalidCredentialsWithoutCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handlers{Service: &fakeService{authErr: business.ErrInvalidCredentials}}
	r := gin.New()
	r.POST("/login", h.Login)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"login":"admin","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatalf("invalid login set cookies: %v", w.Result().Cookies())
	}
}

func TestRequireAuth_ResolvesCookieAndPopulatesContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeService{
		user:    &authDb.User{ID: 3, Login: "admin"},
		session: &authDb.Session{Token: "valid", UserID: 3},
	}
	h := &Handlers{Service: service}
	r := gin.New()
	r.Use(h.RequireAuth())
	r.GET("/me", h.Me)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "valid"})
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || service.resolvedWith != "valid" {
		t.Fatalf("status=%d token=%q body=%s", w.Code, service.resolvedWith, w.Body.String())
	}
}

func TestRequireAuth_RejectsInvalidSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handlers{Service: &fakeService{resolveErr: errors.New("expired")}}
	r := gin.New()
	r.Use(h.RequireAuth())
	r.GET("/protected", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "invalid"})
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestLogout_RevokesSessionAndClearsCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeService{revokeErr: errors.New("db failure")}
	h := &Handlers{Service: service, CookieSameSite: "Strict"}
	r := gin.New()
	r.POST("/logout", h.Logout)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "token"})
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || service.revoked != "token" {
		t.Fatalf("status=%d revoked=%q body=%s", w.Code, service.revoked, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("logout cookie was not cleared: %+v", cookies)
	}
}

func TestIncompleteWiringFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("login", func(t *testing.T) {
		h := &Handlers{}
		r := gin.New()
		r.POST("/login", h.Login)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"login":"admin","password":"secret"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})

	t.Run("middleware", func(t *testing.T) {
		h := &Handlers{}
		r := gin.New()
		r.Use(h.RequireAuth())
		r.GET("/protected", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "token"})
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("logout", func(t *testing.T) {
		h := &Handlers{}
		r := gin.New()
		r.POST("/logout", h.Logout)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/logout", nil)
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "token"})
		r.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}
