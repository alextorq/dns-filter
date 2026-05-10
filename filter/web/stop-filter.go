package web

import (
	"errors"
	"net/http"

	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/filter"
	pausefilter "github.com/alextorq/dns-filter/filter/business/use-cases/pause-filter"
	"github.com/gin-gonic/gin"
)

type FilterStatusResponse struct {
	Status bool `json:"status"`
	// PausedUntil is the unix-second deadline of an active pause, or 0 if no
	// pause is active. The frontend uses this absolute value to drive its
	// countdown without depending on server-supplied "seconds left".
	PausedUntil int64 `json:"paused_until"`
}

type PauseFilterRequest struct {
	Minutes int `json:"minutes"`
}

// ChangeFilterStatus toggles the global DNS filter on/off.
// @Summary      Toggle the DNS filter
// @Tags         filter
// @Produce      json
// @Success      200 {object} FilterStatusResponse
// @Router       /api/filter/change-status [post]
func ChangeFilterStatus(c *gin.Context) {
	val := filter.ChangeFilterDnsRecords()
	c.JSON(http.StatusOK, FilterStatusResponse{Status: val, PausedUntil: filter.GetPausedUntil()})
}

// GetFilterStatus returns whether the DNS filter is enabled.
// @Summary      Get filter status
// @Tags         filter
// @Produce      json
// @Success      200 {object} FilterStatusResponse
// @Router       /api/filter/status [get]
func GetFilterStatus(c *gin.Context) {
	conf := config.GetConfig()
	c.JSON(http.StatusOK, FilterStatusResponse{
		Status:      conf.Enabled.Load(),
		PausedUntil: filter.GetPausedUntil(),
	})
}

// PauseFilter pauses filtering for a fixed number of minutes (5, 10, 15, 30).
// @Summary      Pause the DNS filter for N minutes
// @Tags         filter
// @Accept       json
// @Produce      json
// @Param        request body PauseFilterRequest true "duration in minutes"
// @Success      200 {object} FilterStatusResponse
// @Failure      400 {object} map[string]string
// @Router       /api/filter/pause [post]
func PauseFilter(c *gin.Context) {
	var req PauseFilterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	until, err := filter.PauseFilter(req.Minutes)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, pausefilter.ErrInvalidDuration):
			status = http.StatusBadRequest
		case errors.Is(err, pausefilter.ErrFilterDisabled):
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, FilterStatusResponse{
		Status:      config.GetConfig().Enabled.Load(),
		PausedUntil: until,
	})
}

// ResumeFilter clears any active pause, returning the filter to its toggled state.
// @Summary      Resume the DNS filter (clear pause)
// @Tags         filter
// @Produce      json
// @Success      200 {object} FilterStatusResponse
// @Router       /api/filter/resume [post]
func ResumeFilter(c *gin.Context) {
	filter.ResumeFilter()
	c.JSON(http.StatusOK, FilterStatusResponse{
		Status:      config.GetConfig().Enabled.Load(),
		PausedUntil: 0,
	})
}
