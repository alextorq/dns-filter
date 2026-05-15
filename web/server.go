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

// Handlers bundles every per-feature *Handlers struct that requires
// constructor-injected dependencies. main builds it as the single
// composition-root step for the HTTP API. Feature packages without DI
// register themselves via package-level Register(rg) functions instead.
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
	r := buildRouter(h)

	go func() {
		r.Run(":8080")
	}()

	return r
}

// buildRouter assembles the gin.Engine with every route the API exposes but
// does NOT start listening. Each feature contributes its own paths via
// RegisterRoutes / Register — this function only owns cross-cutting concerns
// (CORS, the public/protected split, Swagger).
func buildRouter(h Handlers) *gin.Engine {
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
	authWeb.RegisterPublic(r)

	// Everything else under /api/* requires a valid session.
	api := r.Group("/api", authWeb.RequireAuth())
	authWeb.Register(api)
	h.Blocked.RegisterRoutes(api)
	h.Filter.RegisterRoutes(api)
	h.Suggest.RegisterRoutes(api)
	h.Source.RegisterRoutes(api)
	clientsWeb.Register(api)
	dbWeb.Register(api)
	dnsCacheWeb.Register(api)
	inspectWeb.Register(api)
	loggerWeb.Register(api)

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}
