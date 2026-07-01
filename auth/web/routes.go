package web

import "github.com/gin-gonic/gin"

// RegisterPublic wires auth endpoints that intentionally sit outside the
// session-protected /api group — namely login, which is the only way in.
func (h *Handlers) RegisterPublic(r gin.IRouter) {
	r.POST("/api/auth/login", h.Login)
}

// RegisterRoutes wires auth endpoints that require an authenticated session. The
// group is expected to already carry authentication middleware.
func (h *Handlers) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/auth/logout", h.Logout)
	rg.GET("/auth/me", h.Me)
}
