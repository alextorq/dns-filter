package web

import "github.com/gin-gonic/gin"

// RegisterRoutes wires runtime-logger HTTP endpoints onto rg. The group is
// expected to already carry authentication middleware.
func (h *Handlers) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/config/logger/change-level", h.ChangeLogLevel)
	rg.POST("/config/logger/get-level", h.GetLogLevelHandler)
}
