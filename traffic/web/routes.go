package web

import "github.com/gin-gonic/gin"

// RegisterRoutes wires every traffic-dashboard HTTP endpoint onto rg. The group
// is expected to already carry authentication middleware (these are protected,
// like the other dashboard features). The per-device domains endpoint
// identifies the device with kind+value QUERY params rather than a path segment
// because a MAC value contains colons that are awkward/ambiguous in a path.
func (h *Handlers) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/traffic/devices", h.GetDevices)
	rg.GET("/traffic/devices/domains", h.GetDeviceDomains)
	rg.GET("/traffic/top-domains", h.GetTopDomains)
}
