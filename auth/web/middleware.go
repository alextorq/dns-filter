package web

import (
	"net/http"

	"github.com/alextorq/dns-filter/auth/business"
	"github.com/gin-gonic/gin"
)

const (
	SessionCookieName = "dnsf_session"
	contextUserKey    = "auth.user"
	contextSessionKey = "auth.session"
)

// RequireAuth aborts the request with 401 when the session cookie is missing
// or invalid. On success, the user is attached to the context.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(SessionCookieName)
		if err != nil || token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
			return
		}

		session, user, err := business.ResolveSession(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
			return
		}

		c.Set(contextSessionKey, session)
		c.Set(contextUserKey, user)
		c.Next()
	}
}
