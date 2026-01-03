package web

import (
	"fmt"
	"net/http"

	dns_records "github.com/alextorq/dns-filter/dns-records"
	createdomain "github.com/alextorq/dns-filter/dns-records/business/use-cases/create-domain"
	"github.com/alextorq/dns-filter/dns-records/business/use-cases/update-dns-record"
	dns_records_db "github.com/alextorq/dns-filter/dns-records/db"
	"github.com/alextorq/dns-filter/logger"
	use_cases "github.com/alextorq/dns-filter/use-cases"
	"github.com/gin-gonic/gin"
)

func GetAllDnsRecords(c *gin.Context) {
	l := logger.GetLogger()
	type RequestBody struct {
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
		Filter string `json:"filter"`
	}

	var req RequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, gin.H{
			"message": err.Error(),
		})
		return
	}

	res, err := dns_records.GetRecordsByFilter(dns_records_db.GetAllParams{
		Limit:  req.Limit,
		Offset: req.Offset,
		Filter: req.Filter,
	})
	if err != nil {
		l.Error(fmt.Errorf("error get dns records from db: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"list":  res.List,
		"total": res.Total,
	})
}

func CreateDnsRecords(c *gin.Context) {
	l := logger.GetLogger()

	var req createdomain.RequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, gin.H{
			"message": err.Error(),
		})
		return
	}

	err := createdomain.CreateDomain(req)

	if err != nil {
		l.Error(fmt.Errorf("error create new dns record: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "domain created",
	})

}

func ChangeDnsRecordActive(c *gin.Context) {
	l := logger.GetLogger()
	var updateData update_dns_record.UpdateBlockList

	if err := c.ShouldBindJSON(&updateData); err != nil {
		l.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, gin.H{
			"message": err.Error(),
		})
		return
	}

	record, err := update_dns_record.UpdateDnsRecord(updateData)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}

	err = use_cases.UpdateFilterFromDb()
	if err != nil {
		l.Error(fmt.Errorf("error update filter from db when change record: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"record": *record,
	})
}
