package web

import (
	"errors"
	"net/http"
	"strings"

	authDb "github.com/alextorq/dns-filter/auth/db"
	"github.com/alextorq/dns-filter/auth/business"
	"github.com/alextorq/dns-filter/config"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

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

func setSessionCookie(c *gin.Context, token string, maxAge int) {
	cfg := config.GetConfig()
	c.SetSameSite(sameSiteFromConfig(cfg.CookieSameSite))
	c.SetCookie(
		SessionCookieName,
		token,
		maxAge,
		"/",
		"",
		cfg.CookieSecure,
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
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	user, err := authDb.GetUserByLogin(req.Login)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	if !business.CheckPassword(user.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid credentials"})
		return
	}

	session, err := business.IssueSession(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	setSessionCookie(c, session.Token, int(business.SessionTTL.Seconds()))
	c.JSON(http.StatusOK, UserResponse{ID: user.ID, Login: user.Login})
}

// Logout revokes the current session and clears the cookie.
// @Summary      Logout
// @Tags         auth
// @Produce      json
// @Success      200  {object} StatusResponse
// @Router       /api/auth/logout [post]
func Logout(c *gin.Context) {
	if token, err := c.Cookie(SessionCookieName); err == nil && token != "" {
		_ = business.RevokeSession(token)
	}
	setSessionCookie(c, "", -1)
	c.JSON(http.StatusOK, StatusResponse{Status: "ok"})
}

// Me returns information about the current user.
// @Summary      Current user
// @Tags         auth
// @Produce      json
// @Success      200  {object} UserResponse
// @Failure      401  {object} ErrorResponse
// @Router       /api/auth/me [get]
func Me(c *gin.Context) {
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
