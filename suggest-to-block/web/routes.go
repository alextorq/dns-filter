package web

import "github.com/gin-gonic/gin"

// RegisterRoutes wires every suggest-to-block HTTP endpoint onto rg. The
// group is expected to already carry authentication middleware.
func (h *Handlers) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/suggest-to-block", h.GetAllSuggestBlocks)
	rg.GET("/suggest-to-block/codes", h.GetSignalCodes)
	rg.POST("/suggest-to-block/add-to-block", h.AddToBlock)
	rg.POST("/suggest-to-block/change-status", h.ChangeActiveStatus)
}
