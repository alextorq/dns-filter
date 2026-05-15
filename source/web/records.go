package web

import (
	"fmt"
	"net/http"

	syncDb "github.com/alextorq/dns-filter/source/db"
	"github.com/gin-gonic/gin"
)

type Logger interface {
	Info(args ...any)
	Error(err error)
}

// BlockRepo is the narrow port over the blocklist that ChangeSourceActive
// needs to mass-toggle the active flag of every blocklist row whose Source
// matches the toggled source name.
type BlockRepo interface {
	ChangeRecordStatusBySource(source string, active bool) error
}

type Filter interface {
	UpdateFromDb() error
}

type Handlers struct {
	Repo      *syncDb.Repo
	BlockRepo BlockRepo
	Filter    Filter
	Log       Logger
}

// GetAllSources lists block-list sources known to the filter.
// @Summary      List block-list sources
// @Tags         sources
// @Produce      json
// @Success      200 {object} GetAllSourcesResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/sources [post]
func (h *Handlers) GetAllSources(c *gin.Context) {
	res, err := h.Repo.GetAll(syncDb.GetAllParams{})
	if err != nil {
		h.Log.Error(fmt.Errorf("error get dns records from db: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GetAllSourcesResponse{
		List:  res,
		Total: h.Repo.Amount(),
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
func (h *Handlers) ChangeSourceActive(c *gin.Context) {
	var req ChangeSourceActiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Log.Error(fmt.Errorf("error bind json when changing source active: %w", err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	source, err := h.Repo.GetByID(req.ID)
	if err != nil {
		h.Log.Error(fmt.Errorf("error get source by id from db: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}
	source.Active = req.Active
	if err := h.Repo.Update(source); err != nil {
		h.Log.Error(fmt.Errorf("error update source active in db: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	if err := h.BlockRepo.ChangeRecordStatusBySource(source.Name.String(), req.Active); err != nil {
		h.Log.Error(fmt.Errorf("error change blocked domains by source in db: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	if err := h.Filter.UpdateFromDb(); err != nil {
		h.Log.Error(fmt.Errorf("error update filter after changing source active: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, ChangeSourceActiveResponse{
		Message: "record updated",
		Record:  source,
	})
}
