package web

import (
	"net/http"

	"github.com/alextorq/dns-filter/config"
	change_filter_dns_records "github.com/alextorq/dns-filter/use-cases/change-filter-dns-records"
	"github.com/gin-gonic/gin"
)

func ChangeFilterStatus(c *gin.Context) {
	val := change_filter_dns_records.ChangeFilterDnsRecords()
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
