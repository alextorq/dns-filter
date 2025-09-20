package web

import (
	"fmt"
	"net/http"

	blacklists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/logger"
	"github.com/alextorq/dns-filter/use-cases/update-dns-record"
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

	res, err := blacklists.GetRecordsByFilter(blacklists.GetAllParams{
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
	list, err := blacklists.GetAllActive()
	if err != nil {
		l.Error(fmt.Errorf("error get dns records from db: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"list": list,
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

	c.JSON(http.StatusOK, gin.H{
		"record": *record,
	})
}
