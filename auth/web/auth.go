package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/alextorq/dns-filter/auth/business"
	authDb "github.com/alextorq/dns-filter/auth/db"
	"github.com/gin-gonic/gin"
)

type Service interface {
	Authenticate(login, password string) (*authDb.User, *authDb.Session, error)
	ResolveSession(token string) (*authDb.Session, *authDb.User, error)
	RevokeSession(token string) error
}

type Handlers struct {
	Service        Service
	CookieSecure   bool
	CookieSameSite string
}

func sameSiteFromConfig(s string) http.SameSite {
	switch strings.ToLower(s) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func (h *Handlers) setSessionCookie(c *gin.Context, token string, maxAge int) {
	c.SetSameSite(sameSiteFromConfig(h.CookieSameSite))
	c.SetCookie(
		SessionCookieName,
		token,
		maxAge,
		"/",
		"",
		h.CookieSecure,
		true,
	)
}

// Login authenticates a user and issues a session cookie.
// @Summary      Login
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body body     LoginRequest true "Credentials"
// @Success      200  {object} UserResponse
// @Failure      400  {object} ErrorResponse
// @Failure      401  {object} ErrorResponse
// @Router       /api/auth/login [post]
func (h *Handlers) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if h == nil || h.Service == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "authentication unavailable"})
		return
	}

	user, session, err := h.Service.Authenticate(req.Login, req.Password)
	if err != nil {
		if errors.Is(err, business.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	h.setSessionCookie(c, session.Token, int(business.SessionTTL.Seconds()))
	c.JSON(http.StatusOK, UserResponse{ID: user.ID, Login: user.Login})
}

// Logout revokes the current session and clears the cookie.
// @Summary      Logout
// @Tags         auth
// @Produce      json
// @Success      200  {object} StatusResponse
// @Router       /api/auth/logout [post]
func (h *Handlers) Logout(c *gin.Context) {
	if token, err := c.Cookie(SessionCookieName); err == nil && token != "" {
		if h == nil || h.Service == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "authentication unavailable"})
			return
		}
		_ = h.Service.RevokeSession(token)
	}
	h.setSessionCookie(c, "", -1)
	c.JSON(http.StatusOK, StatusResponse{Status: "ok"})
}

// Me returns information about the current user.
// @Summary      Current user
// @Tags         auth
// @Produce      json
// @Success      200  {object} UserResponse
// @Failure      401  {object} ErrorResponse
// @Router       /api/auth/me [get]
func (h *Handlers) Me(c *gin.Context) {
	v, ok := c.Get(contextUserKey)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}
	user, ok := v.(*authDb.User)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, UserResponse{ID: user.ID, Login: user.Login})
}
