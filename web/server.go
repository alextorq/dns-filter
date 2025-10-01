package web

import (
	"github.com/alextorq/dns-filter/black-lists/web"
	eventsWeb "github.com/alextorq/dns-filter/blocked-domain/web"
	configWeb "github.com/alextorq/dns-filter/config/web"
	filterWeb "github.com/alextorq/dns-filter/filter/web"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CreateSever() *gin.Engine {
	gin.SetMode(gin.ReleaseMode) // или gin.DebugMode
	r := gin.Default()
	//TODO: configure CORS
	r.Use(cors.Default())

	r.POST("/api/dns-records", web.GetAllDnsRecords)
	r.POST("/api/dns-records/create", web.CreateDnsRecords)
	r.POST("/api/dns-records/update", web.ChangeDnsRecordActive)

	r.GET("/api/filter/status", filterWeb.GetFilterStatus)
	r.POST("/api/filter/change-status", filterWeb.ChangeFilterStatus)

	r.POST("/api/events/block/amount", eventsWeb.GetAmount)
	r.POST("/api/events/block/amount-by-group", eventsWeb.GetAmountByDomain)

	r.POST("/api/config/logger/change-level", configWeb.ChangeLogLevel)
	r.POST("/api/config/logger/get-level", configWeb.GetLogLevel)

	go func() {
		r.Run(":8080")
	}()

	return r
}
