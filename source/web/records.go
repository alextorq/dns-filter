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

func GetAllSources(c *gin.Context) {
	l := logger.GetLogger()

	res, err := syncDb.GetAllRecords(syncDb.GetAllParams{})
	if err != nil {
		l.Error(fmt.Errorf("error get dns records from db: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}

	count := syncDb.GetAmountRecords()

	c.JSON(http.StatusOK, gin.H{
		"list":  res,
		"total": count,
	})
}

func ChangeSourceActive(c *gin.Context) {
	l := logger.GetLogger()
	type RequestBody struct {
		ID     uint `json:"id"`
		Active bool `json:"active"`
	}

	var req RequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		l.Error(fmt.Errorf("error bind json when changing source active: %w", err))
		c.JSON(http.StatusBadRequest, gin.H{
			"message": err.Error(),
		})
		return
	}

	source, err := syncDb.GetRecordByID(req.ID)
	if err != nil {
		l.Error(fmt.Errorf("error get source by id from db: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}
	source.Active = req.Active
	syncDb.UpdateRecord(source)
	if err != nil {
		l.Error(fmt.Errorf("error update source active in db: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}

	blDb.ChangeRecordStatusBySource(source.Name.String(), req.Active)
	if err != nil {
		l.Error(fmt.Errorf("error change blocked domains by source in db: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}

	err = filter.UpdateFilterFromDb()

	if err != nil {
		l.Error(fmt.Errorf("error update filter after changing source active: %w", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "record updated",
		"record":  source,
	})
}
