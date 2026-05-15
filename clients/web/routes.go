package web

import "github.com/gin-gonic/gin"

// Register wires every clients HTTP endpoint onto rg. The group is expected
// to already carry authentication middleware.
func Register(rg *gin.RouterGroup) {
	rg.POST("/clients", ListClients)
	rg.POST("/clients/create", CreateClient)
	rg.POST("/clients/update", UpdateClient)
	rg.POST("/clients/change-filter", ChangeFilter)
	rg.POST("/clients/delete", DeleteClient)
	rg.POST("/clients/discover", Discover)
}
