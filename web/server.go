package web

import (
	authWeb "github.com/alextorq/dns-filter/auth/web"
	eventsWeb "github.com/alextorq/dns-filter/blocked-domain/web"
	clientsWeb "github.com/alextorq/dns-filter/clients/web"
	dbWeb "github.com/alextorq/dns-filter/db/web"
	dnsCacheWeb "github.com/alextorq/dns-filter/dns-cache/web"
	_ "github.com/alextorq/dns-filter/docs"
	inspectWeb "github.com/alextorq/dns-filter/domain-inspect/web"
	filterWeb "github.com/alextorq/dns-filter/filter/web"
	loggerWeb "github.com/alextorq/dns-filter/logger/web"
	syncWeb "github.com/alextorq/dns-filter/source/web"
	suggestWeb "github.com/alextorq/dns-filter/suggest-to-block/web"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Handlers bundles every per-feature *Handlers struct the HTTP layer wires up.
// main builds it as the single composition-root step for the HTTP API.
type Handlers struct {
	Blocked *eventsWeb.Handlers
	Filter  *filterWeb.Handlers
	Suggest *suggestWeb.Handlers
	Source  *syncWeb.Handlers
}

// CreateServer wires HTTP routes onto a fresh gin.Engine and starts it on
// :8080 in a goroutine. All per-feature dependencies are injected via the
// Handlers bundle — this function reads no singletons.
func CreateServer(h Handlers) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
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

		api.POST("/dns-records", h.Blocked.GetAllDnsRecords)
		api.POST("/dns-records/create", h.Blocked.CreateDnsRecords)
		api.POST("/dns-records/update", h.Blocked.ChangeDnsRecordActive)

		api.GET("/filter/status", h.Filter.GetFilterStatus)
		api.POST("/filter/change-status", h.Filter.ChangeFilterStatus)
		api.POST("/filter/pause", h.Filter.PauseFilter)
		api.POST("/filter/resume", h.Filter.ResumeFilter)

		api.POST("/events/block/amount", h.Blocked.GetAmount)
		api.POST("/events/block/amount-by-group", h.Blocked.GetAmountByDomain)

		api.POST("/suggest-to-block", h.Suggest.GetAllSuggestBlocks)
		api.GET("/suggest-to-block/codes", suggestWeb.GetSignalCodes)
		api.POST("/suggest-to-block/add-to-block", h.Suggest.AddToBlock)
		api.POST("/suggest-to-block/change-status", h.Suggest.ChangeActiveStatus)

		api.POST("/config/logger/change-level", loggerWeb.ChangeLogLevel)
		api.POST("/config/logger/get-level", loggerWeb.GetLogLevel)

		api.POST("/sources", h.Source.GetAllSources)
		api.POST("/sources/change-status", h.Source.ChangeSourceActive)

		api.POST("/clients", clientsWeb.ListClients)
		api.POST("/clients/create", clientsWeb.CreateClient)
		api.POST("/clients/update", clientsWeb.UpdateClient)
		api.POST("/clients/change-filter", clientsWeb.ChangeFilter)
		api.POST("/clients/delete", clientsWeb.DeleteClient)
		api.POST("/clients/discover", clientsWeb.Discover)

		api.GET("/config/db/download", dbWeb.DownloadDb)

		api.POST("/dns-cache/clear", dnsCacheWeb.ClearCache)

		api.GET("/domain/inspect", inspectWeb.Inspect)
	}

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	go func() {
		r.Run(":8080")
	}()

	return r
}
