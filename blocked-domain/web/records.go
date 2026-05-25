package web

import (
	"errors"
	"fmt"
	"net/http"

	create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	update_dns_record "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/update-dns-record"
	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	syncDb "github.com/alextorq/dns-filter/source/db"
	"github.com/gin-gonic/gin"
)

// Logger is the narrow logging port used by handlers.
type Logger interface {
	Info(args ...any)
	Error(err error)
}

// BlockStatsRepo is the narrow read port behind the legacy block-stats
// endpoints (/api/events/block/*). Step 4 of the traffic-dashboard migration
// repoints these reads off the soon-to-be-removed block_domain_events table
// onto the unified domain_traffic counter: BlockedTotalCount is SUM(count)
// WHERE blocked, BlockedCountByDomain the same grouped by domain. The response
// JSON shape is unchanged, so the existing frontend is unaffected.
// *traffic/db.Repo is adapted to this port at the composition root.
type BlockStatsRepo interface {
	BlockedTotalCount() (int64, error)
	BlockedCountByDomain() ([]blocked_domain_db.DomainCount, error)
}

// Handlers groups the blocked-domain HTTP endpoints with their dependencies.
// Construct one at the composition root and reuse it across route
// registrations; do not call package-level helpers.
type Handlers struct {
	Repo          *blocked_domain_db.Repo
	Log           Logger
	RefreshFilter func() error
	// BlockStats backs the /api/events/block/* stats endpoints. Injected
	// separately from Repo so the block-list CRUD path and the (now
	// traffic-backed) stats path depend on distinct, narrow ports.
	BlockStats BlockStatsRepo
}

// GetAllDnsRecords returns a paginated, filterable slice of the block list.
// @Summary      List blocked DNS records
// @Tags         dns-records
// @Accept       json
// @Produce      json
// @Param        body body     GetAllDnsRecordsRequest true "Pagination and filtering"
// @Success      200  {object} GetAllDnsRecordsResponse
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/dns-records [post]
func (h *Handlers) GetAllDnsRecords(c *gin.Context) {
	var req GetAllDnsRecordsRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		h.Log.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	res, err := h.Repo.GetRecordsByFilter(blocked_domain_db.GetAllParams{
		Limit:  req.Limit,
		Offset: req.Offset,
		Filter: req.Filter,
		Source: req.Source,
	})
	if err != nil {
		h.Log.Error(fmt.Errorf("error get dns records from db: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, GetAllDnsRecordsResponse{
		List:  res.List,
		Total: res.Total,
	})
}

// CreateDnsRecords adds a user-managed domain to the block list.
// @Summary      Create a blocked DNS record
// @Tags         dns-records
// @Accept       json
// @Produce      json
// @Param        body body     create_domain.RequestBody true "Domain to block"
// @Success      200  {object} MessageResponse
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/dns-records/create [post]
func (h *Handlers) CreateDnsRecords(c *gin.Context) {
	var req create_domain.RequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		h.Log.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	err := create_domain.CreateDomain(
		create_domain.Deps{Repo: h.Repo, Log: h.Log},
		create_domain.RequestBody{
			Domain: req.Domain,
			Source: syncDb.SourceUser.String(),
		},
	)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, create_domain.ErrEmptyDomain) {
			status = http.StatusBadRequest
		}
		h.Log.Error(fmt.Errorf("error create new dns record: %w", err))
		c.JSON(status, ErrorResponse{Message: err.Error()})
		return
	}

	if err := h.RefreshFilter(); err != nil {
		h.Log.Error(fmt.Errorf("error refresh filter after create: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "domain created"})
}

// ChangeDnsRecordActive toggles the active flag on a block-list record.
// @Summary      Update a blocked DNS record
// @Tags         dns-records
// @Accept       json
// @Produce      json
// @Param        body body     update_dns_record.UpdateBlockList true "Record id and target active state"
// @Success      200  {object} UpdateDnsRecordResponse
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/dns-records/update [post]
func (h *Handlers) ChangeDnsRecordActive(c *gin.Context) {
	var updateData update_dns_record.UpdateBlockList

	if err := c.ShouldBindJSON(&updateData); err != nil {
		h.Log.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	record, err := update_dns_record.UpdateDnsRecord(
		update_dns_record.Deps{Repo: h.Repo, Log: h.Log},
		updateData,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	if err := h.RefreshFilter(); err != nil {
		h.Log.Error(fmt.Errorf("error refresh filter after update: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	h.Log.Info("Record updated:")

	c.JSON(http.StatusOK, UpdateDnsRecordResponse{
		Message: "record updated",
		Record:  record,
	})
}
