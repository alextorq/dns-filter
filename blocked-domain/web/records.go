package web

import (
	"fmt"
	"net/http"

	"github.com/alextorq/dns-filter/blocked-domain"
	create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	update_dns_record "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/update-dns-record"
	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/logger"
	syncDb "github.com/alextorq/dns-filter/source/db"
	"github.com/gin-gonic/gin"
)

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
func GetAllDnsRecords(c *gin.Context) {
	l := logger.GetLogger()

	var req GetAllDnsRecordsRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	res, err := blocked_domain.GetRecordsByFilter(blocked_domain_db.GetAllParams{
		Limit:  req.Limit,
		Offset: req.Offset,
		Filter: req.Filter,
		Source: req.Source,
	})
	if err != nil {
		l.Error(fmt.Errorf("error get dns records from db: %w", err))
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
func CreateDnsRecords(c *gin.Context) {
	l := logger.GetLogger()

	var req create_domain.RequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	err := blocked_domain.CreateDomain(create_domain.RequestBody{
		Domain: req.Domain,
		Source: syncDb.SourceUser.String(),
	})

	if err != nil {
		l.Error(fmt.Errorf("error create new dns record: %w", err))
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
func ChangeDnsRecordActive(c *gin.Context) {
	l := logger.GetLogger()
	var updateData update_dns_record.UpdateBlockList

	if err := c.ShouldBindJSON(&updateData); err != nil {
		l.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	record, err := blocked_domain.UpdateDnsRecord(updateData)

	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	l.Info("Record updated:")

	c.JSON(http.StatusOK, UpdateDnsRecordResponse{
		Message: "record updated",
		Record:  record,
	})
}
