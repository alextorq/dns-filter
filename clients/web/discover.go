package web

import (
	"context"
	"net/http"

	"github.com/alextorq/dns-filter/clients/discovery"
	"github.com/alextorq/dns-filter/config"
	"github.com/gin-gonic/gin"
)

// DiscoverResponse is the wire shape of POST /api/clients/discover.
//
// Discovery is best-effort: a partial failure (e.g., active ARP scan denied
// because the container lacks CAP_NET_RAW) returns 200 with the entries we
// did manage to collect plus a non-empty Errors list. The frontend shows the
// errors as a banner above the table so the operator knows why coverage is
// incomplete.
type DiscoverResponse struct {
	Devices []discovery.Device `json:"devices"`
	Total   int                `json:"total"`
	Errors  []string           `json:"errors,omitempty"`
}

// Discover scans the LAN for devices.
// @Summary      Scan LAN for devices
// @Tags         clients
// @Accept       json
// @Produce      json
// @Param        body body     DiscoverRequest false "Scan options"
// @Success      200 {object} DiscoverResponse
// @Failure      400 {object} BadRequestResponse "malformed request body"
// @Failure      409 {object} ErrorResponse "discovery is not supported in the current deployment mode"
// @Failure      500 {object} ErrorResponse
// @Router       /api/clients/discover [post]
func Discover(c *gin.Context) {
	// Discovery is meaningless in public mode — there is no LAN around the
	// server, just whatever subnet the cloud provider assigned. Refuse early
	// with a structured error rather than returning a misleading empty list.
	if config.GetConfig().Mode != config.ModeLAN {
		c.JSON(http.StatusConflict, ErrorResponse{
			Error: "LAN discovery is only available in LAN deployment mode",
		})
		return
	}

	// The body is optional (the scanner runs with no options too). But when a
	// body IS sent it must be valid: a malformed or wrong-typed field is a
	// client error, not silently the default — same contract as CreateClient.
	var req DiscoverRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, BadRequestResponse{Message: err.Error()})
			return
		}
	}
	// Absent field → hide Docker (matches the UI checkbox default).
	filterDocker := boolOr(req.FilterDocker, true)

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	res, err := discovery.Discover(ctx, discovery.DiscoverOptions{FilterDocker: filterDocker})
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, DiscoverResponse{
		Devices: res.Devices,
		Total:   res.Total,
		Errors:  res.Errors,
	})
}
