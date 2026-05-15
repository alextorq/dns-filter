package web

import "github.com/gin-gonic/gin"

// RegisterRoutes wires every blocked-domain HTTP endpoint onto rg. The group
// is expected to already carry authentication middleware.
func (h *Handlers) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/dns-records", h.GetAllDnsRecords)
	rg.POST("/dns-records/create", h.CreateDnsRecords)
	rg.POST("/dns-records/update", h.ChangeDnsRecordActive)

	rg.POST("/events/block/amount", h.GetAmount)
	rg.POST("/events/block/amount-by-group", h.GetAmountByDomain)
}
