// @title           DNS Filter API
// @version         1.0
// @description     HTTP API for the DNS filter: managing block lists, sources, clients, suggestions and runtime config.
// @BasePath        /
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	authBusiness "github.com/alextorq/dns-filter/auth/business"
	auth_db "github.com/alextorq/dns-filter/auth/db"
	authWeb "github.com/alextorq/dns-filter/auth/web"
	"github.com/alextorq/dns-filter/background"
	background_sourcesync "github.com/alextorq/dns-filter/background/sourcesync"
	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	blockedWeb "github.com/alextorq/dns-filter/blocked-domain/web"
	"github.com/alextorq/dns-filter/clients"
	"github.com/alextorq/dns-filter/clients/arpwatcher"
	clients_db "github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/discovery"
	"github.com/alextorq/dns-filter/clients/hostnames"
	hostnames_db "github.com/alextorq/dns-filter/clients/hostnames/db"
	"github.com/alextorq/dns-filter/clients/identifier"
	clients_store "github.com/alextorq/dns-filter/clients/store"
	clientsWeb "github.com/alextorq/dns-filter/clients/web"
	"github.com/alextorq/dns-filter/config"
	app_db "github.com/alextorq/dns-filter/db"
	"github.com/alextorq/dns-filter/db/migrate"
	db_web "github.com/alextorq/dns-filter/db/web"
	"github.com/alextorq/dns-filter/dns"
	dns_cache "github.com/alextorq/dns-filter/dns-cache"
	dns_cache_web "github.com/alextorq/dns-filter/dns-cache/web"
	domain_inspect_checks "github.com/alextorq/dns-filter/domain-inspect/checks"
	domainInspectWeb "github.com/alextorq/dns-filter/domain-inspect/web"
	"github.com/alextorq/dns-filter/filter"
	filter_cache "github.com/alextorq/dns-filter/filter/cache"
	filter_bloom "github.com/alextorq/dns-filter/filter/filter"
	filter_state "github.com/alextorq/dns-filter/filter/runtime-state"
	filterWeb "github.com/alextorq/dns-filter/filter/web"
	"github.com/alextorq/dns-filter/logger"
	consoleHandler "github.com/alextorq/dns-filter/logger/handlers/console"
	loggerWeb "github.com/alextorq/dns-filter/logger/web"
	"github.com/alextorq/dns-filter/metric"
	"github.com/alextorq/dns-filter/settings"
	settings_db "github.com/alextorq/dns-filter/settings/db"
	settingsWeb "github.com/alextorq/dns-filter/settings/web"
	"github.com/alextorq/dns-filter/source"
	source_sync "github.com/alextorq/dns-filter/source/business/use-cases/sync"
	source_db "github.com/alextorq/dns-filter/source/db"
	sourceWeb "github.com/alextorq/dns-filter/source/web"
	suggest_to_block "github.com/alextorq/dns-filter/suggest-to-block"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
	suggest_inspect "github.com/alextorq/dns-filter/suggest-to-block/inspect"
	inspect_db "github.com/alextorq/dns-filter/suggest-to-block/inspect/db"
	suggestWeb "github.com/alextorq/dns-filter/suggest-to-block/web"
	traffic_prune_uc "github.com/alextorq/dns-filter/traffic/business/use-cases/prune"
	traffic_record_uc "github.com/alextorq/dns-filter/traffic/business/use-cases/record"
	traffic_db "github.com/alextorq/dns-filter/traffic/db"
	trafficWeb "github.com/alextorq/dns-filter/traffic/web"
	"github.com/alextorq/dns-filter/web"
	"github.com/prometheus/client_golang/prometheus"
)

// buildIdentifier picks the per-request client identifier strategy based on
// the deployment Mode. ModePublic is reserved for the future DoH frontend; we
// fall through to the LAN strategy today so a misconfigured public deploy
// still answers queries instead of silently failing every lookup.
//
// In LAN mode the IPIdentifier is wired with the arpwatcher cache so the
// hot path can resolve incoming IP → MAC and consult the exclusion store
// by MAC (which survives DHCP IP rotation). Before the watcher's first
// refresh the cache is empty, so identification falls back to IP — that's
// the same behavior as PR1 and is correct (rules just haven't migrated to
// MAC-keyed yet).
func buildIdentifier(mode config.Mode, resolver identifier.MACResolver) identifier.Identifier {
	switch mode {
	case config.ModePublic:
		return identifier.IPIdentifier{}
	case config.ModeLAN:
		fallthrough
	default:
		return identifier.IPIdentifier{Resolver: resolver}
	}
}

// serverLogger is the narrow logging port used by server error reporting.
type serverLogger interface {
	Error(err error)
}

// reportServerError suppresses the expected Shutdown result and surfaces every
// real listener/runtime failure through the application logger.
func reportServerError(name string, err error, log serverLogger) {
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error(fmt.Errorf("%s server stopped: %w", name, err))
	}
}

func main() {
	conf := config.Load()
	chanLogger := logger.NewChanLogger(1000, conf.LogLevel)
	chanLogger.AddHandler(&consoleHandler.ConsoleHandler{})
	registry := prometheus.NewRegistry()
	dbMetrics, err := app_db.NewMetrics(registry)
	if err != nil {
		panic(fmt.Errorf("create DB metrics: %w", err))
	}
	conn, err := app_db.Open(app_db.OpenDeps{
		Path:    conf.DbPath,
		Log:     chanLogger,
		Metrics: dbMetrics,
		DBName:  "main",
	})
	if err != nil {
		panic(fmt.Errorf("open database: %w", err))
	}
	migrate.Migrate(conn)
	if err := metric.RegisterRuntimeCollectors(registry, chanLogger.DroppedCount); err != nil {
		chanLogger.Error(fmt.Errorf("register runtime metrics: %w", err))
	}
	authModule := authBusiness.NewModule(auth_db.NewRepo(conn), conf.AdminLogin, conf.AdminPassword)
	if err := authModule.BootstrapAdmin(); err != nil {
		panic(err)
	}

	// Composition root for the DI-enabled features: each gets its own *Repo over
	// the single connection, then *Module / *Handlers wired from those repos.
	// Domain-inspect checks and their runtime credentials are also constructed
	// explicitly below; feature code does not resolve application dependencies.
	blockRepo := blocked_domain_db.NewRepo(conn)
	sourceRepo := source_db.NewRepo(conn)
	suggestRepo := suggest_to_block_db.NewRepo(conn)
	settingsRepo := settings_db.NewRepo(conn)
	trafficRepo := traffic_db.NewRepo(conn)
	hostnamesRepo := hostnames_db.NewRepo(conn)
	clientRepo := clients_db.NewRepo(conn)

	bloom := filter_bloom.NewFilter()
	cache := filter_cache.NewCacheWithMetrics(1500)
	filterRuntimeState := filter_state.New(true)
	filterModule := filter.NewModule(blockRepo, bloom, cache, filterRuntimeState, chanLogger)

	sourceHTTPClient := &http.Client{Timeout: 60 * time.Second}
	sourceLoaders, err := source_sync.NewDefaultLoaders(sourceHTTPClient)
	if err != nil {
		panic(fmt.Errorf("create source loaders: %w", err))
	}
	sourceModule, err := source.NewModule(sourceRepo, blockRepo, sourceLoaders, chanLogger)
	if err != nil {
		panic(fmt.Errorf("create source module: %w", err))
	}
	sourceModule.Seed()

	// Populate the bloom from whatever the DB already holds so the DNS server
	// can answer queries immediately, without waiting on the network. On a
	// genuine first run the DB is empty and nothing is blocked until the
	// background sync below finishes — the trade-off for a non-blocking start.
	if err := filterModule.UpdateFromDb(); err != nil {
		panic(err)
	}
	clientStore := clients_store.New()
	clientModule := clients.NewModule(clientRepo, clientStore, discovery.NewDefaultScanner())
	if err := clientModule.Sync(); err != nil {
		panic(err)
	}
	arpCache := arpwatcher.NewCache()

	// suggest-to-block and domain-inspect READ allowed-domain data from the
	// unified domain_traffic counter (domains ever forwarded upstream). The
	// legacy allow_domain_events table has been removed; these ports now read
	// only from traffic.
	trafficAllowAdapter := traffic_db.NewAllowFilterAdapter(trafficRepo)
	suggestModule := suggest_to_block.NewModule(blockRepo, trafficAllowAdapter, sourceRepo, filterModule, suggestRepo, chanLogger)
	inspectCredentials := domain_inspect_checks.NewCredentials()
	inspectEnabled := suggest_inspect.NewEnabledState()
	inspectCatalog := domain_inspect_checks.NewDefaultCatalog(domain_inspect_checks.CatalogDeps{
		Blocks:      blockRepo,
		Allowed:     trafficRepo,
		Credentials: inspectCredentials,
		URLScanKey:  conf.URLScanKey,
	})

	// Reputation-enrichment worker. Подключается всегда — мастер-тогл
	// (suggest_inspect_enabled) и API-ключи (virustotal_key, safebrowsing_key)
	// теперь — DB-настройки и могут включаться/выключаться без рестарта.
	//
	// inspectGate композирует два сигнала: внедрённый мастер-тогл фичи
	// И наличие хотя бы одного провайдер-ключа (inspectCredentials.HasAnyKey). Без ключа
	// RDAP/urlscan/dns_resolve всё равно стучатся наружу за каждым кандидатом
	// и сливают наблюдённые домены LAN в публичные сервисы, а VT/SB отдают
	// «skipped» — пользы ноль. Поэтому фича считается active, только когда оба
	// условия истинны. Тот же gate используется и Worker.RunOnce, и
	// suggestModule.Collect (отказ маршрутизировать в очередь).
	//
	// Запуск горутин suggest/inspect перенесён ниже HydrateAll, чтобы оба
	// runtime-состояния уже соответствовали БД-override до первого тика; иначе zero-value
	// gate (false) погасил бы первый Collect/RunOnce даже при включённой фиче.
	inspectRepo := inspect_db.NewRepo(conn)
	inspectMetrics, err := suggest_inspect.NewMetrics(registry)
	if err != nil {
		panic(fmt.Errorf("create inspect metrics: %w", err))
	}
	suggestModule.SetInspectQueue(inspectRepo)
	inspectGate := func() bool { return inspectEnabled.Enabled() && inspectCredentials.HasAnyKey() }
	suggestModule.SetInspectGate(inspectGate)
	inspectAdapter := suggest_inspect.NewAdapter(inspectRepo, conf.SuggestInspectCacheTTL, suggest_inspect.ProviderChecks{
		RDAP:         inspectCatalog.RDAP,
		VirusTotal:   inspectCatalog.VirusTotal,
		SafeBrowsing: inspectCatalog.SafeBrowsing,
	}, inspectMetrics)
	inspectWorker := suggest_inspect.NewWorker(
		inspectRepo, inspectAdapter, blockRepo, suggestRepo, sourceRepo, filterModule, chanLogger, inspectMetrics,
		suggest_inspect.WorkerConfig{
			Budget:    conf.SuggestInspectBudget,
			Interval:  conf.SuggestInspectInterval,
			CacheTTL:  conf.SuggestInspectCacheTTL,
			Pause:     conf.SuggestInspectPause,
			Backoff:   conf.SuggestInspectBackoff,
			MaxErrors: conf.SuggestInspectMaxErrors,
		},
	)
	inspectWorker.SetFeatureGate(inspectGate)

	backgroundJobs := []background.Job{
		background.JobFunc(func(ctx context.Context) {
			authModule.ClearExpiredSessions(ctx, chanLogger)
		}),
	}
	dbSizeMonitor, err := app_db.NewDBSizeMonitor(
		registry,
		conf.DbPath,
		chanLogger,
		app_db.DefaultDBSizeMonitorInterval,
	)
	if err != nil {
		chanLogger.Error(fmt.Errorf("create DB size monitor: %w", err))
	} else {
		backgroundJobs = append(backgroundJobs, dbSizeMonitor)
	}
	// Start the ARP watcher only in LAN mode. Public mode has no LAN to
	// observe; the watcher would just spam ErrUnsupported (or, in a hosted
	// environment with /proc/net/arp present, learn meaningless cloud-VLAN
	// pairs). The watcher exits its own loop on non-Linux platforms.
	if conf.Mode == config.ModeLAN {
		arpWatcher := arpwatcher.NewWatcher(arpCache, clientRepo, clientModule.Sync)
		backgroundJobs = append(backgroundJobs, background.JobFunc(func(ctx context.Context) {
			arpWatcher.Run(ctx, chanLogger, arpwatcher.DefaultInterval)
		}))

		// Background mDNS sweep that learns friendly device names and persists
		// them as MAC→hostname rows. It resolves discovered IPs to MACs via the
		// same arpwatcher cache the hot path uses, so a device's traffic rows and
		// its learned name share the stable MAC key. LAN-only: there is no LAN to
		// browse behind a public DoH endpoint.
		hostnameCollector := &hostnames.Collector{
			Browse: discovery.BrowseMDNS,
			MACs:   arpCache,
			Store:  hostnamesRepo,
			Log:    chanLogger,
		}
		backgroundJobs = append(backgroundJobs, hostnameCollector)
	}

	cacheMetrics, err := dns_cache.NewMetrics(registry)
	if err != nil {
		panic(fmt.Errorf("create DNS cache metrics: %w", err))
	}
	cacheWithMetric := dns_cache.NewCacheWithMetricsAndSWR(1500, conf.CacheStaleGrace, conf.CacheStaleTTL, cacheMetrics)
	dnsMetrics, err := dns.NewMetrics(registry)
	if err != nil {
		panic(fmt.Errorf("create DNS metrics: %w", err))
	}
	// Per-device traffic counter (the unified table). It is the sole recorder of
	// block/allow verdicts now that the legacy event stores are gone. Capacity
	// bounds DISTINCT aggregation keys held in RAM between flushes, not raw
	// events, so it can be sized generously.
	trafficWorker := traffic_record_uc.NewTrafficEventStore(trafficRepo, chanLogger, 2000)
	trafficRetention := traffic_prune_uc.NewRetentionState()

	// Reloadable upstream: constructed from env defaults, then re-pointed by the
	// settings hydrate below if a DB override exists. The same instance backs
	// both the hot path and the SWR refresh worker, so a runtime swap repoints
	// both at once.
	resolver := dns.NewReloadableResolver(conf.DoHUpstream, conf.DoHBootstrapIPs...)

	ident := buildIdentifier(conf.Mode, arpCache)
	dnsServer := dns.NewServer(dns.ServerDeps{
		Logger:             chanLogger,
		Cache:              cacheWithMetric,
		Filter:             filterModule.CheckExist,
		Metric:             dnsMetrics,
		Identifier:         ident,
		Clients:            clientStore,
		Upstream:           resolver,
		SWREnabled:         conf.CacheSWR,
		RefreshConcurrency: conf.CacheRefreshConcurrency,
	})
	dnsServer.Traffic = trafficWorker

	// Runtime settings store. Every sink (logger, resolver, cache, server) now
	// exists, so we declare the DB-backed settings, restore the persisted filter
	// toggle, and hydrate effective values into the running process — all before
	// dnsServer.Serve() starts accepting queries.
	settingsModule := settings.NewModule(settingsRepo)
	registerDynamicSettings(settingsModule, dynamicSettingsDeps{
		conf:               conf,
		logr:               chanLogger,
		resolver:           resolver,
		cache:              cacheWithMetric,
		dnsServer:          dnsServer,
		inspectEnabled:     inspectEnabled,
		inspectCredentials: inspectCredentials,
		trafficRetention:   trafficRetention,
	})
	filterModule.SetStateSink(filter.PersistHook(settingsRepo, chanLogger))
	if err := filter.RestoreState(settingsRepo, filterRuntimeState); err != nil {
		// Non-fatal: a failed restore leaves the filter at its compiled default
		// (enabled) rather than aborting an otherwise-healthy boot.
		chanLogger.Error(fmt.Errorf("restore filter state: %w", err))
	}
	if err := settingsModule.HydrateAll(); err != nil {
		// Non-fatal: HydrateAll already substituted defaults for any bad rows;
		// this just reports what it skipped.
		chanLogger.Error(fmt.Errorf("settings hydrate: %w", err))
	}

	// Запускаем suggest- и inspect-горутины только после HydrateAll: к этому
	// моменту атомик suggest_inspect_enabled и injected VT/SB credentials соответствуют
	// БД-override, и inspectGate в первом же Collect/RunOnce читает их свежими.
	// Иначе первый Module.Collect выполнялся бы с zero-value атомика (false) и
	// дропал weak-band кандидатов даже при включённой фиче — следующий шанс был
	// бы через Interval. StartPrune работает независимо, обслуживая retention
	// `inspect_candidate`/`rdap_cache` даже когда воркер пассивен.
	backgroundJobs = append(backgroundJobs,
		background.JobFunc(suggestModule.Start),
		background.JobFunc(inspectWorker.Start),
		background.JobFunc(func(ctx context.Context) {
			suggest_inspect.StartPrune(ctx, inspectRepo, 4*conf.SuggestInspectCacheTTL, chanLogger)
		}),
	)

	// Daily retention prune over the unified domain_traffic table — the sole
	// retention task (the two legacy block/allow clear-events tasks were removed
	// with their tables). The retention window is the traffic_retention_days
	// dynamic setting; the loop reads its atomic fresh each tick, so a UI change
	// applies on the next prune. Launched AFTER HydrateAll so the very first
	// (immediate) prune already sees the effective window — otherwise it could
	// hard-delete rows using the pre-hydrate seed (see traffic_prune.RetentionState).
	backgroundJobs = append(backgroundJobs, background.JobFunc(func(ctx context.Context) {
		traffic_prune_uc.Run(ctx, trafficRepo, trafficRetention, chanLogger)
	}))

	// Pull the block lists in the background and refresh the filter once done.
	// The DNS server (started below via dnsServer.Serve) does not wait on this.
	// Launched after HydrateAll so the persisted log level is already applied
	// when the source sync job emits its "started" line — otherwise that INFO line
	// races ahead of hydrate and prints even when the level was raised to WARN,
	// while the matching "finished" line (logged later, post-hydrate) is
	// suppressed, making a healthy sync look stuck.
	sourceSyncJob, err := background_sourcesync.New(sourceModule, filterModule, chanLogger)
	if err != nil {
		panic(fmt.Errorf("create source sync job: %w", err))
	}
	backgroundJobs = append(backgroundJobs, sourceSyncJob)

	backgroundRunner, err := background.NewRunner(backgroundJobs...)
	if err != nil {
		panic(fmt.Errorf("create background runner: %w", err))
	}
	backgroundCtx := context.Background()
	go backgroundRunner.Run(backgroundCtx)

	// Start metrics only after every component-specific collector has been
	// registered. main keeps the handle for the common shutdown lifecycle.
	var metricsServer *http.Server
	if conf.MetricEnable {
		metricsAddr := ":" + conf.MetricPort
		metricsServer = metric.NewServer(metricsAddr, registry)
		chanLogger.Info("Метрики Prometheus доступны на", metricsAddr+"/metrics")
		go func(server *http.Server) {
			reportServerError("metrics", server.ListenAndServe(), chanLogger)
		}(metricsServer)
	}

	httpServer := web.NewServer(":8080", web.Handlers{
		Auth: &authWeb.Handlers{
			Service:        authModule,
			CookieSecure:   conf.CookieSecure,
			CookieSameSite: conf.CookieSameSite,
		},
		Clients: &clientsWeb.Handlers{
			Service: clientModule,
			Log:     chanLogger,
			Mode:    conf.Mode,
		},
		DNSCache: &dns_cache_web.Handlers{
			Cache: cacheWithMetric,
			Log:   chanLogger,
		},
		Inspect: domainInspectWeb.NewHandlers(inspectCatalog.Checks, chanLogger),
		Blocked: &blockedWeb.Handlers{
			Records:       blockRepo,
			Creator:       blockRepo,
			Updater:       blockRepo,
			Log:           chanLogger,
			RefreshFilter: filterModule.UpdateFromDb,
			// Step 4: legacy block-stats endpoints now read SUM(count) WHERE
			// blocked from domain_traffic instead of block_domain_events.
			BlockStats: traffic_db.NewBlockStatsAdapter(trafficRepo),
		},
		Filter: &filterWeb.Handlers{Module: filterModule},
		Suggest: &suggestWeb.Handlers{
			Repo:      suggestRepo,
			BlockRepo: blockRepo,
			Filter:    filterModule,
			Log:       chanLogger,
		},
		Source: &sourceWeb.Handlers{
			Repo:      sourceRepo,
			BlockRepo: blockRepo,
			Filter:    filterModule,
			Log:       chanLogger,
		},
		Logger: &loggerWeb.Handlers{
			SetLogLevel: func(level string) error { return settingsModule.Set("log_level", level) },
			GetLogLevel: chanLogger.GetLogLevel,
		},
		Settings: &settingsWeb.Handlers{Service: settingsModule},
		Database: &db_web.Handlers{
			DB:         conn,
			DBPath:     conf.DbPath,
			Log:        chanLogger,
			SecretKeys: settingsModule.SecretKeys,
		},
		// Per-device traffic dashboard (read-only). Vendor enrichment uses the
		// pure, local OUI lookup; hostname enrichment reads the mDNS collector's
		// MAC→hostname table (empty in public mode, where no collector runs).
		Traffic: trafficWeb.NewHandlers(trafficRepo, discovery.LookupVendor, hostnamesRepo.AllAsMap, chanLogger),
	})
	go func() {
		reportServerError("HTTP", httpServer.ListenAndServe(), chanLogger)
	}()

	if err := dnsServer.Serve(); err != nil {
		panic(err)
	}
}
