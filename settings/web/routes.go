package web

import "github.com/gin-gonic/gin"

// RegisterRoutes wires the settings endpoints onto rg. The group is expected
// to already carry authentication middleware.
func (h *Handlers) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/settings", h.ListSettings)
	rg.PUT("/settings/:key", h.UpdateSetting)
	rg.DELETE("/settings/:key", h.ResetSetting)
}
