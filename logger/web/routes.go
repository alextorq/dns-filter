package web

import "github.com/gin-gonic/gin"

// Register wires runtime-logger HTTP endpoints onto rg. The group is
// expected to already carry authentication middleware.
func Register(rg *gin.RouterGroup) {
	rg.POST("/config/logger/change-level", ChangeLogLevel)
	rg.POST("/config/logger/get-level", GetLogLevel)
}
