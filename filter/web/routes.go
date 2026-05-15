package web

import "github.com/gin-gonic/gin"

// RegisterRoutes wires every filter HTTP endpoint onto rg. The group is
// expected to already carry authentication middleware.
func (h *Handlers) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/filter/status", h.GetFilterStatus)
	rg.POST("/filter/change-status", h.ChangeFilterStatus)
	rg.POST("/filter/pause", h.PauseFilter)
	rg.POST("/filter/resume", h.ResumeFilter)
}
