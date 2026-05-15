package web

import "github.com/gin-gonic/gin"

// Register wires DNS-cache HTTP endpoints onto rg. The group is expected to
// already carry authentication middleware.
func Register(rg *gin.RouterGroup) {
	rg.POST("/dns-cache/clear", ClearCache)
}
