package web

import (
	"fmt"
	"net/http"

	blDb "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/filter"
	"github.com/alextorq/dns-filter/logger"
	syncDb "github.com/alextorq/dns-filter/source/db"
	"github.com/gin-gonic/gin"
)

// GetAllSources lists block-list sources known to the filter.
// @Summary      List block-list sources
// @Tags         sources
// @Produce      json
// @Success      200 {object} GetAllSourcesResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/sources [post]
func GetAllSources(c *gin.Context) {
	l := logger.GetLogger()

	res, err := syncDb.GetAllRecords(syncDb.GetAllParams{})
	if err != nil {
		l.Error(fmt.Errorf("error get dns records from db: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	count := syncDb.GetAmountRecords()

	c.JSON(http.StatusOK, GetAllSourcesResponse{
		List:  res,
		Total: count,
	})
}

// ChangeSourceActive toggles a source on/off and refreshes the in-memory filter.
// @Summary      Toggle a block-list source
// @Tags         sources
// @Accept       json
// @Produce      json
// @Param        body body     ChangeSourceActiveRequest true "Source id and target active state"
// @Success      200  {object} ChangeSourceActiveResponse
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/sources/change-status [post]
func ChangeSourceActive(c *gin.Context) {
	l := logger.GetLogger()

	var req ChangeSourceActiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when changing source active: %w", err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	source, err := syncDb.GetRecordByID(req.ID)
	if err != nil {
		l.Error(fmt.Errorf("error get source by id from db: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}
	source.Active = req.Active
	syncDb.UpdateRecord(source)
	if err != nil {
		l.Error(fmt.Errorf("error update source active in db: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	blDb.ChangeRecordStatusBySource(source.Name.String(), req.Active)
	if err != nil {
		l.Error(fmt.Errorf("error change blocked domains by source in db: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	err = filter.UpdateFilterFromDb()

	if err != nil {
		l.Error(fmt.Errorf("error update filter after changing source active: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, ChangeSourceActiveResponse{
		Message: "record updated",
		Record:  source,
	})
}
