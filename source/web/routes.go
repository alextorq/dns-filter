package web

import "github.com/gin-gonic/gin"

// RegisterRoutes wires every source HTTP endpoint onto rg. The group is
// expected to already carry authentication middleware.
func (h *Handlers) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/sources", h.GetAllSources)
	rg.POST("/sources/change-status", h.ChangeSourceActive)
}
