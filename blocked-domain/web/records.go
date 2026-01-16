package web

import (
	"fmt"
	"net/http"

	"github.com/alextorq/dns-filter/blocked-domain"
	create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	update_dns_record "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/update-dns-record"
	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/logger"
	"github.com/gin-gonic/gin"
)

func GetAllDnsRecords(c *gin.Context) {
	l := logger.GetLogger()
	type RequestBody struct {
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
		Filter string `json:"filter"`
		Source string `json:"source"`
	}

	var req RequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, gin.H{
			"message": err.Error(),
		})
		return
	}

	res, err := blocked_domain.GetRecordsByFilter(blocked_domain_db.GetAllParams{
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

	var req create_domain.RequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when change record: %w", err))
		c.JSON(http.StatusBadRequest, gin.H{
			"message": err.Error(),
		})
		return
	}

	err := blocked_domain.CreateDomain(create_domain.RequestBody{
		Domain: req.Domain,
		Source: blocked_domain_db.SourceUser,
	})

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

	record, err := blocked_domain.UpdateDnsRecord(updateData)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}

	l.Info("Record updated:")

	c.JSON(http.StatusOK, gin.H{
		"message": "record updated",
		"record":  record,
	})
}
