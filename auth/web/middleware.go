package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	SessionCookieName = "dnsf_session"
	contextUserKey    = "auth.user"
	contextSessionKey = "auth.session"
)

// RequireAuth aborts the request with 401 when the session cookie is missing
// or invalid. On success, the user is attached to the context.
func (h *Handlers) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h == nil || h.Service == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
			return
		}
		token, err := c.Cookie(SessionCookieName)
		if err != nil || token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
			return
		}

		session, user, err := h.Service.ResolveSession(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
			return
		}

		c.Set(contextSessionKey, session)
		c.Set(contextUserKey, user)
		c.Next()
	}
}
