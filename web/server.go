package web

import (
	authWeb "github.com/alextorq/dns-filter/auth/web"
	eventsWeb "github.com/alextorq/dns-filter/blocked-domain/web"
	clientsWeb "github.com/alextorq/dns-filter/clients/web"
	dbWeb "github.com/alextorq/dns-filter/db/web"
	_ "github.com/alextorq/dns-filter/docs"
	filterWeb "github.com/alextorq/dns-filter/filter/web"
	loggerWeb "github.com/alextorq/dns-filter/logger/web"
	syncWeb "github.com/alextorq/dns-filter/source/web"
	suggestWeb "github.com/alextorq/dns-filter/suggest-to-block/web"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func CreateServer() *gin.Engine {
	gin.SetMode(gin.ReleaseMode) // или gin.DebugMode
	r := gin.Default()
	//TODO: configure CORS
	r.Use(cors.New(cors.Config{
		AllowOriginFunc:  func(origin string) bool { return true },
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowCredentials: true,
	}))

	// Public auth endpoints — login is the only way in.
	r.POST("/api/auth/login", authWeb.Login)

	// Everything else under /api/* requires a valid session.
	api := r.Group("/api", authWeb.RequireAuth())
	{
		api.POST("/auth/logout", authWeb.Logout)
		api.GET("/auth/me", authWeb.Me)

		api.POST("/dns-records", eventsWeb.GetAllDnsRecords)
		api.POST("/dns-records/create", eventsWeb.CreateDnsRecords)
		api.POST("/dns-records/update", eventsWeb.ChangeDnsRecordActive)

		api.GET("/filter/status", filterWeb.GetFilterStatus)
		api.POST("/filter/change-status", filterWeb.ChangeFilterStatus)
		api.POST("/filter/pause", filterWeb.PauseFilter)
		api.POST("/filter/resume", filterWeb.ResumeFilter)

		api.POST("/events/block/amount", eventsWeb.GetAmount)
		api.POST("/events/block/amount-by-group", eventsWeb.GetAmountByDomain)

		api.POST("/suggest-to-block", suggestWeb.GetAllSuggestBlocks)
		api.GET("/suggest-to-block/codes", suggestWeb.GetSignalCodes)
		api.POST("/suggest-to-block/add-to-block", suggestWeb.AddToBlock)
		api.POST("/suggest-to-block/change-status", suggestWeb.ChangeActiveStatus)

		api.POST("/config/logger/change-level", loggerWeb.ChangeLogLevel)
		api.POST("/config/logger/get-level", loggerWeb.GetLogLevel)

		api.POST("/sources", syncWeb.GetAllSources)
		api.POST("/sources/change-status", syncWeb.ChangeSourceActive)

		api.POST("/clients", clientsWeb.ListClients)
		api.POST("/clients/create", clientsWeb.CreateClient)
		api.POST("/clients/update", clientsWeb.UpdateClient)
		api.POST("/clients/change-filter", clientsWeb.ChangeFilter)
		api.POST("/clients/delete", clientsWeb.DeleteClient)

		api.GET("/config/db/download", dbWeb.DownloadDb)
	}

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	go func() {
		r.Run(":8080")
	}()

	return r
}
