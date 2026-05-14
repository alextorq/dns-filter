package web

import (
	authWeb "github.com/alextorq/dns-filter/auth/web"
	blockeddb "github.com/alextorq/dns-filter/blocked-domain/db"
	eventsWeb "github.com/alextorq/dns-filter/blocked-domain/web"
	clientsWeb "github.com/alextorq/dns-filter/clients/web"
	app_db "github.com/alextorq/dns-filter/db"
	dbWeb "github.com/alextorq/dns-filter/db/web"
	dnsCacheWeb "github.com/alextorq/dns-filter/dns-cache/web"
	_ "github.com/alextorq/dns-filter/docs"
	inspectWeb "github.com/alextorq/dns-filter/domain-inspect/web"
	"github.com/alextorq/dns-filter/filter"
	filterWeb "github.com/alextorq/dns-filter/filter/web"
	"github.com/alextorq/dns-filter/logger"
	loggerWeb "github.com/alextorq/dns-filter/logger/web"
	syncWeb "github.com/alextorq/dns-filter/source/web"
	suggestWeb "github.com/alextorq/dns-filter/suggest-to-block/web"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// CreateServer wires HTTP routes onto a fresh gin.Engine and starts it on
// :8080 in a goroutine.
//
// MUST be called AFTER migrate.Migrate(): the composition root below
// constructs a Repo over the open *gorm.DB, and Repo methods assume the
// schema already exists. If you ever rearrange main.go to start HTTP earlier
// (e.g. to expose a /healthz before the bloom load), pass the *gorm.DB in
// from main instead of pulling it from the singleton here.
func CreateServer() *gin.Engine {
	gin.SetMode(gin.ReleaseMode) // или gin.DebugMode
	r := gin.Default()

	// Composition root for blocked-domain handlers (pilot for the singleton →
	// DI migration). The other features still talk to singletons directly;
	// they will be converted in follow-up PRs.
	blockRepo := blockeddb.NewRepo(app_db.GetConnection())
	blockedHandlers := &eventsWeb.Handlers{
		Repo:          blockRepo,
		Log:           logger.GetLogger(),
		RefreshFilter: filter.UpdateFilterFromDb,
	}
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

		api.POST("/dns-records", blockedHandlers.GetAllDnsRecords)
		api.POST("/dns-records/create", blockedHandlers.CreateDnsRecords)
		api.POST("/dns-records/update", blockedHandlers.ChangeDnsRecordActive)

		api.GET("/filter/status", filterWeb.GetFilterStatus)
		api.POST("/filter/change-status", filterWeb.ChangeFilterStatus)
		api.POST("/filter/pause", filterWeb.PauseFilter)
		api.POST("/filter/resume", filterWeb.ResumeFilter)

		api.POST("/events/block/amount", blockedHandlers.GetAmount)
		api.POST("/events/block/amount-by-group", blockedHandlers.GetAmountByDomain)

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
