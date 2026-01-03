package web

import (
	"fmt"
	"net/http"

	"github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/logger"
	"github.com/gin-gonic/gin"
)

func GetAmount(c *gin.Context) {
	amount := db.GetAmountRows()

	c.JSON(http.StatusOK, gin.H{
		"amount": amount,
	})
}

func GetAmountByDomain(c *gin.Context) {
	l := logger.GetLogger()
	groups, err := db.GetRowsByDomains()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve data",
		})
		l.Error(fmt.Errorf("failed to get rows by domains: %w", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"groups": groups,
	})
}
