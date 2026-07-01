package web

import "github.com/gin-gonic/gin"

// RegisterRoutes wires database-related HTTP endpoints onto rg. The group is
// expected to already carry authentication middleware.
func (h *Handlers) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/config/db/download", h.DownloadDb)
}
