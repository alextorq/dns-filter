package web

import "github.com/gin-gonic/gin"

// Register wires domain-inspect HTTP endpoints onto rg. The group is
// expected to already carry authentication middleware.
func Register(rg *gin.RouterGroup) {
	rg.GET("/domain/inspect", Inspect)
}
