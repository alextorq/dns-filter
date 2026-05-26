package web

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetAmount returns the total number of blocked DNS queries. Step 4 repoints
// this off block_domain_events onto the unified domain_traffic counter
// (SUM(count) WHERE blocked) via the BlockStats port; the {amount} response
// shape is unchanged.
// @Summary      Total block events
// @Tags         events
// @Produce      json
// @Success      200 {object} GetAmountResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/events/block/amount [post]
func (h *Handlers) GetAmount(c *gin.Context) {
	amount, err := h.BlockStats.BlockedTotalCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Failed to retrieve data"})
		h.Log.Error(fmt.Errorf("failed to get blocked total: %w", err))
		return
	}
	c.JSON(http.StatusOK, GetAmountResponse{Amount: amount})
}
