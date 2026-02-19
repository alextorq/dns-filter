package web

import (
	eventsWeb "github.com/alextorq/dns-filter/blocked-domain/web"
	excludeClientsWeb "github.com/alextorq/dns-filter/clients/web"
	dbWeb "github.com/alextorq/dns-filter/db/web"
	filterWeb "github.com/alextorq/dns-filter/filter/web"
	loggerWeb "github.com/alextorq/dns-filter/logger/web"
	syncWeb "github.com/alextorq/dns-filter/source/web"
	suggestWeb "github.com/alextorq/dns-filter/suggest-to-block/web"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CreateServer() *gin.Engine {
	gin.SetMode(gin.ReleaseMode) // или gin.DebugMode
	r := gin.Default()
	//TODO: configure CORS
	r.Use(cors.Default())

	r.POST("/api/dns-records", eventsWeb.GetAllDnsRecords)
	r.POST("/api/dns-records/create", eventsWeb.CreateDnsRecords)
	r.POST("/api/dns-records/update", eventsWeb.ChangeDnsRecordActive)

	r.GET("/api/filter/status", filterWeb.GetFilterStatus)
	r.POST("/api/filter/change-status", filterWeb.ChangeFilterStatus)

	r.POST("/api/events/block/amount", eventsWeb.GetAmount)
	r.POST("/api/events/block/amount-by-group", eventsWeb.GetAmountByDomain)

	r.POST("/api/suggest-to-block", suggestWeb.GetAllSuggestBlocks)
	r.POST("/api/suggest-to-block/add-to-block", suggestWeb.AddToBlock)
	r.POST("/api/suggest-to-block/change-status", suggestWeb.ChangeActiveStatus)

	r.POST("/api/config/logger/change-level", loggerWeb.ChangeLogLevel)
	r.POST("/api/config/logger/get-level", loggerWeb.GetLogLevel)

	r.POST("/api/sources", syncWeb.GetAllSources)
	r.POST("/api/sources/change-status", syncWeb.ChangeSourceActive)

	r.POST("/api/exclude-clients", excludeClientsWeb.GetAllClients)
	r.POST("/api/exclude-clients/add", excludeClientsWeb.AddClient)
	r.POST("/api/exclude-clients/change-status", excludeClientsWeb.ChangeClientStatus)
	r.POST("/api/exclude-clients/delete", excludeClientsWeb.DeleteClient)

	r.GET("/api/config/db/download", dbWeb.DownloadDb)

	go func() {
		r.Run(":8080")
	}()

	return r
}
