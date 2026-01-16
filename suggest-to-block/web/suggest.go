package web

import (
	"fmt"
	"net/http"

	"github.com/alextorq/dns-filter/blocked-domain"
	blocked_domain_use_cases_create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/filter"
	"github.com/alextorq/dns-filter/logger"
	"github.com/alextorq/dns-filter/suggest-to-block"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
	"github.com/gin-gonic/gin"
)

func GetAllSuggestBlocks(c *gin.Context) {
	l := logger.GetLogger()
	type RequestBody struct {
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
		Filter string `json:"filter"`
		Active *bool  `json:"active"`
	}

	var req RequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when getting suggest blocks: %w", err))
		c.JSON(http.StatusBadRequest, gin.H{
			"message": err.Error(),
		})
		return
	}

	res, err := suggest_to_block.GetRecordsByFilter(suggest_to_block_db.GetAllParams{
		Limit:  req.Limit,
		Offset: req.Offset,
		Filter: req.Filter,
		Active: req.Active,
	})
	if err != nil {
		l.Error(fmt.Errorf("error get suggest blocks from db: %w", err))
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

func AddToBlock(c *gin.Context) {
	l := logger.GetLogger()
	type RequestBody struct {
		ID     uint   `json:"id"`
		Domain string `json:"domain"`
	}

	var req RequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json suggest block: %w", err))
		c.JSON(http.StatusBadRequest, gin.H{
			"message": err.Error(),
		})
		return
	}

	err := blocked_domain.CreateDomain(blocked_domain_use_cases_create_domain.RequestBody{
		Domain: req.Domain,
		Source: blocked_domain_db.SourceSuggestedToBlock,
	})

	if err != nil {
		l.Error(fmt.Errorf("error change status suggest block: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}

	err = suggest_to_block.ChangeActiveStatus(req.ID, false)
	if err != nil {
		l.Error(fmt.Errorf("error change status suggest block: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}

	err = filter.UpdateFilterFromDb()

	if err != nil {
		l.Error(fmt.Errorf("error change status suggest block: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "suggest block add to blocklist",
	})
}

func ChangeActiveStatus(c *gin.Context) {
	l := logger.GetLogger()
	type RequestBody struct {
		ID     uint `json:"id"`
		Active bool `json:"active"`
	}

	var req RequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when change suggest block status: %w", err))
		c.JSON(http.StatusBadRequest, gin.H{
			"message": err.Error(),
		})
		return
	}

	err := suggest_to_block.ChangeActiveStatus(req.ID, req.Active)

	if err != nil {
		l.Error(fmt.Errorf("error change suggest block status: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "suggest block status changed",
	})
}
