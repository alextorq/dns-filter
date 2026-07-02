package web

import "github.com/gin-gonic/gin"

// Register wires every clients HTTP endpoint onto rg. The group is expected
// to already carry authentication middleware.
func (h *Handlers) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/clients", h.ListClients)
	rg.POST("/clients/create", h.CreateClient)
	rg.POST("/clients/update", h.UpdateClient)
	rg.POST("/clients/change-filter", h.ChangeFilter)
	rg.POST("/clients/delete", h.DeleteClient)
	rg.POST("/clients/discover", h.Discover)
}
