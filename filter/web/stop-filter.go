package web

import (
	"net/http"

	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/filter"
	"github.com/gin-gonic/gin"
)

func ChangeFilterStatus(c *gin.Context) {
	val := filter.ChangeFilterDnsRecords()
	c.JSON(http.StatusOK, gin.H{
		"status": val,
	})
}

func GetFilterStatus(c *gin.Context) {
	conf := config.GetConfig()

	c.JSON(http.StatusOK, gin.H{
		"status": conf.Enabled,
	})
}
