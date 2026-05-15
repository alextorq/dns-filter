package web

import (
	"errors"
	"fmt"
	"net/http"

	create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	source_db "github.com/alextorq/dns-filter/source/db"
	collect "github.com/alextorq/dns-filter/suggest-to-block/business/use-cases/collect"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
	"github.com/gin-gonic/gin"
)

// BlockRepo is the narrow port over the blocklist that AddToBlock needs.
// *blocked-domain/db.Repo satisfies it.
type BlockRepo interface {
	DomainNotExist(domain string) bool
	CreateDomain(domain, source string) error
}

type SuggestRepo interface {
	GetByFilter(params suggest_to_block_db.GetAllParams) (*suggest_to_block_db.GetAllResult, error)
	UpdateActive(id uint, active bool) error
}

type Filter interface {
	UpdateFromDb() error
}

type Logger interface {
	Info(args ...any)
	Error(err error)
}

// Handlers groups the suggest-to-block HTTP endpoints with their dependencies.
type Handlers struct {
	Repo      SuggestRepo
	BlockRepo BlockRepo
	Filter    Filter
	Log       Logger
}

// GetAllSuggestBlocks lists collected domain suggestions awaiting moderation.
// @Summary      List suggested-to-block domains
// @Tags         suggest-to-block
// @Accept       json
// @Produce      json
// @Param        body body     GetAllSuggestBlocksRequest true "Pagination and filtering"
// @Success      200  {object} GetAllSuggestBlocksResponse
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/suggest-to-block [post]
func (h *Handlers) GetAllSuggestBlocks(c *gin.Context) {
	var req GetAllSuggestBlocksRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		h.Log.Error(fmt.Errorf("error bind json when getting suggest blocks: %w", err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	res, err := h.Repo.GetByFilter(suggest_to_block_db.GetAllParams{
		Limit:  req.Limit,
		Offset: req.Offset,
		Filter: req.Filter,
		Active: req.Active,
		Codes:  req.Codes,
	})
	if err != nil {
		h.Log.Error(fmt.Errorf("error get suggest blocks from db: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, GetAllSuggestBlocksResponse{
		List:  res.List,
		Total: res.Total,
	})
}

// AddToBlock promotes a suggestion into the active block list.
// @Summary      Promote suggestion to block list
// @Tags         suggest-to-block
// @Accept       json
// @Produce      json
// @Param        body body     AddToBlockRequest true "Suggestion id and domain"
// @Success      200  {object} MessageResponse
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/suggest-to-block/add-to-block [post]
func (h *Handlers) AddToBlock(c *gin.Context) {
	var req AddToBlockRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		h.Log.Error(fmt.Errorf("error bind json suggest block: %w", err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	err := create_domain.CreateDomain(
		create_domain.Deps{Repo: h.BlockRepo, Log: h.Log},
		create_domain.RequestBody{
			Domain: req.Domain,
			Source: source_db.SourceSuggestedToBlock.String(),
		},
	)

	alreadyBlocked := errors.Is(err, create_domain.ErrDomainAlreadyExists)
	if err != nil && !alreadyBlocked {
		h.Log.Error(fmt.Errorf("error create domain from suggest: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	if err := h.Repo.UpdateActive(req.ID, false); err != nil {
		h.Log.Error(fmt.Errorf("error change status suggest block: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	if alreadyBlocked {
		c.JSON(http.StatusOK, MessageResponse{Message: "domain already in blocklist, suggestion deactivated"})
		return
	}

	if err := h.Filter.UpdateFromDb(); err != nil {
		h.Log.Error(fmt.Errorf("error update filter after add to block: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "suggest block add to blocklist"})
}

// GetSignalCodes returns the catalog of reason codes with human-readable
// labels and descriptions. Frontend uses it to render the multi-select
// filter and to map codes → labels in the table.
// @Summary      List reason codes
// @Tags         suggest-to-block
// @Produce      json
// @Success      200  {object} GetSignalCodesResponse
// @Router       /api/suggest-to-block/codes [get]
func GetSignalCodes(c *gin.Context) {
	c.JSON(http.StatusOK, GetSignalCodesResponse{List: collect.Catalog()})
}

// ChangeActiveStatus toggles a suggestion's active flag.
// @Summary      Toggle suggestion active state
// @Tags         suggest-to-block
// @Accept       json
// @Produce      json
// @Param        body body     ChangeSuggestStatusRequest true "Suggestion id and target active state"
// @Success      200  {object} MessageResponse
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/suggest-to-block/change-status [post]
func (h *Handlers) ChangeActiveStatus(c *gin.Context) {
	var req ChangeSuggestStatusRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		h.Log.Error(fmt.Errorf("error bind json when change suggest block status: %w", err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	if err := h.Repo.UpdateActive(req.ID, req.Active); err != nil {
		h.Log.Error(fmt.Errorf("error change suggest block status: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "suggest block status changed"})
}
