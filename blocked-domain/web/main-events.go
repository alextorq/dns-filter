package web

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetAmount returns the total number of recorded block events.
// @Summary      Total block events
// @Tags         events
// @Produce      json
// @Success      200 {object} GetAmountResponse
// @Router       /api/events/block/amount [post]
func (h *Handlers) GetAmount(c *gin.Context) {
	amount := h.Repo.GetEventsAmount()
	c.JSON(http.StatusOK, GetAmountResponse{Amount: amount})
}

// GetAmountByDomain returns block-event counts grouped by domain.
// @Summary      Block events grouped by domain
// @Tags         events
// @Produce      json
// @Success      200 {object} GetAmountByDomainResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/events/block/amount-by-group [post]
func (h *Handlers) GetAmountByDomain(c *gin.Context) {
	groups, err := h.Repo.GetEventsByDomain()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Failed to retrieve data"})
		h.Log.Error(fmt.Errorf("failed to get rows by domains: %w", err))
		return
	}

	c.JSON(http.StatusOK, GetAmountByDomainResponse{Groups: groups})
}
