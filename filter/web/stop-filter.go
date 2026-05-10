package web

import (
	"net/http"

	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/filter"
	"github.com/gin-gonic/gin"
)

type FilterStatusResponse struct {
	Status bool `json:"status"`
}

// ChangeFilterStatus toggles the global DNS filter on/off.
// @Summary      Toggle the DNS filter
// @Tags         filter
// @Produce      json
// @Success      200 {object} FilterStatusResponse
// @Router       /api/filter/change-status [post]
func ChangeFilterStatus(c *gin.Context) {
	val := filter.ChangeFilterDnsRecords()
	c.JSON(http.StatusOK, FilterStatusResponse{Status: val})
}

// GetFilterStatus returns whether the DNS filter is enabled.
// @Summary      Get filter status
// @Tags         filter
// @Produce      json
// @Success      200 {object} FilterStatusResponse
// @Router       /api/filter/status [get]
func GetFilterStatus(c *gin.Context) {
	conf := config.GetConfig()

	c.JSON(http.StatusOK, FilterStatusResponse{Status: conf.Enabled.Load()})
}
