package web

import (
	"fmt"
	"net/http"

	"github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/logger"
	"github.com/gin-gonic/gin"
)

// GetAmount returns the total number of recorded block events.
// @Summary      Total block events
// @Tags         events
// @Produce      json
// @Success      200 {object} GetAmountResponse
// @Router       /api/events/block/amount [post]
func GetAmount(c *gin.Context) {
	amount := db.GetAmountRows()

	c.JSON(http.StatusOK, GetAmountResponse{Amount: amount})
}

// GetAmountByDomain returns block-event counts grouped by domain.
// @Summary      Block events grouped by domain
// @Tags         events
// @Produce      json
// @Success      200 {object} GetAmountByDomainResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/events/block/amount-by-group [post]
func GetAmountByDomain(c *gin.Context) {
	l := logger.GetLogger()
	groups, err := db.GetRowsByDomains()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Failed to retrieve data"})
		l.Error(fmt.Errorf("failed to get rows by domains: %w", err))
		return
	}

	c.JSON(http.StatusOK, GetAmountByDomainResponse{Groups: groups})
}
