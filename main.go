// @title           DNS Filter API
// @version         1.0
// @description     HTTP API for the DNS filter: managing block lists, sources, clients, suggestions and runtime config.
// @BasePath        /
package main

import (
	"context"
	"fmt"
	"time"

	allow_domain_use_cases "github.com/alextorq/dns-filter/allow-domain/business/use-cases"
	allow_clear_events_uc "github.com/alextorq/dns-filter/allow-domain/business/use-cases/clear-events"
	allow_domain_db "github.com/alextorq/dns-filter/allow-domain/db"
	authBusiness "github.com/alextorq/dns-filter/auth/business"
	block_domain_uc "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/block-domain"
	clear_events_uc "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/clear-events"
	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	blockedWeb "github.com/alextorq/dns-filter/blocked-domain/web"
	"github.com/alextorq/dns-filter/clients"
	"github.com/alextorq/dns-filter/clients/arpwatcher"
	"github.com/alextorq/dns-filter/clients/identifier"
	"github.com/alextorq/dns-filter/config"
	app_db "github.com/alextorq/dns-filter/db"
	"github.com/alextorq/dns-filter/db/migrate"
	"github.com/alextorq/dns-filter/dns"
	dns_cache "github.com/alextorq/dns-filter/dns-cache"
	domain_inspect_checks "github.com/alextorq/dns-filter/domain-inspect/checks"
	"github.com/alextorq/dns-filter/filter"
	filter_cache "github.com/alextorq/dns-filter/filter/cache"
	filter_bloom "github.com/alextorq/dns-filter/filter/filter"
	filterWeb "github.com/alextorq/dns-filter/filter/web"
	"github.com/alextorq/dns-filter/logger"
	loggerWeb "github.com/alextorq/dns-filter/logger/web"
	"github.com/alextorq/dns-filter/settings"
	settings_db "github.com/alextorq/dns-filter/settings/db"
	settingsWeb "github.com/alextorq/dns-filter/settings/web"
	"github.com/alextorq/dns-filter/source"
	source_db "github.com/alextorq/dns-filter/source/db"
	sourceWeb "github.com/alextorq/dns-filter/source/web"
	suggest_to_block "github.com/alextorq/dns-filter/suggest-to-block"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
	suggestWeb "github.com/alextorq/dns-filter/suggest-to-block/web"
	traffic_record_uc "github.com/alextorq/dns-filter/traffic/business/use-cases/record"
	traffic_db "github.com/alextorq/dns-filter/traffic/db"
	"github.com/alextorq/dns-filter/web"
	dnsLib "github.com/miekg/dns"
)

type Handlers struct {
	allowHandler func(domain string)
	blockHandler func(domain string)
}

func (h Handlers) Allowed(_ dnsLib.ResponseWriter, r *dnsLib.Msg) {
	first := r.Question[0]
	domain := first.Name
	h.allowHandler(domain)
}

func (h Handlers) Blocked(_ dnsLib.ResponseWriter, r *dnsLib.Msg) {
	first := r.Question[0]
	domain := first.Name
	h.blockHandler(domain)
}

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
func buildIdentifier(mode config.Mode) identifier.Identifier {
	switch mode {
	case config.ModePublic:
		return identifier.IPIdentifier{}
	case config.ModeLAN:
		fallthrough
	default:
		return identifier.IPIdentifier{Resolver: arpwatcher.Get()}
	}
}

// syncLogger is the narrow logging port backgroundSync needs.
type syncLogger interface {
	Info(args ...any)
	Error(err error)
}

// Retry backoff for the startup source sync: a failed sync (typically no
// network on first boot) is retried with exponential backoff so the sinkhole
// eventually loads its block lists without a process restart. The delay starts
// at syncRetryBaseDelay and doubles up to syncRetryMaxDelay.
const (
	syncRetryBaseDelay = 30 * time.Second
	syncRetryMaxDelay  = 30 * time.Minute
)

// backgroundSync pulls the block lists and, on success, rebuilds the in-memory
// filter (UpdateFromDb refreshes the bloom and clears the verdict cache so a
// freshly blocked domain is not served from a stale verdict).
//
// It is launched as a goroutine after the DNS server is already serving, so —
// unlike the synchronous startup path — it never panics: a panic here would
// take down a DNS server that is already answering traffic. A failed sync is
// retried with exponential backoff (see syncRetryBaseDelay) until it succeeds;
// in the meantime the server keeps running on whatever the DB already held.
func backgroundSync(sync, refresh func() error, log syncLogger) {
	runBackgroundSync(sync, refresh, log, time.Sleep)
}

// runBackgroundSync is backgroundSync with an injectable sleep so the retry
// backoff is testable without real-time delays.
func runBackgroundSync(sync, refresh func() error, log syncLogger, sleep func(time.Duration)) {
	log.Info("Фоновая синхронизация источников запущена")

	delay := syncRetryBaseDelay
	for attempt := 1; ; attempt++ {
		err := sync()
		if err == nil {
			break
		}
		log.Error(fmt.Errorf("фоновая синхронизация источников не удалась (попытка %d), повтор через %s: %w", attempt, delay, err))
		sleep(delay)
		delay = min(delay*2, syncRetryMaxDelay)
	}

	if err := refresh(); err != nil {
		log.Error(fmt.Errorf("обновление фильтра после фоновой синхронизации не удалось: %w", err))
		return
	}
	log.Info("Фоновая синхронизация источников завершена, фильтр обновлён")
}

func main() {
	migrate.Migrate()
	if err := authBusiness.BootstrapAdmin(); err != nil {
		panic(err)
	}

	conn := app_db.GetConnection()
	conf := config.GetConfig()
	chanLogger := logger.GetLogger()

	// Composition root: every feature gets its own *Repo over the single
	// connection, then *Module / *Handlers wired from those repos. After this
	// point no feature reads db.GetConnection() — wiring is explicit.
	blockRepo := blocked_domain_db.NewRepo(conn)
	allowRepo := allow_domain_db.NewRepo(conn)
	sourceRepo := source_db.NewRepo(conn)
	suggestRepo := suggest_to_block_db.NewRepo(conn)
	settingsRepo := settings_db.NewRepo(conn)
	trafficRepo := traffic_db.NewRepo(conn)

	bloom := filter_bloom.GetFilter()
	cache := filter_cache.GetCache()
	filterModule := filter.NewModule(blockRepo, bloom, cache, conf, chanLogger)

	sourceModule := source.NewModule(sourceRepo, blockRepo, chanLogger)
	sourceModule.Seed()

	// Populate the bloom from whatever the DB already holds so the DNS server
	// can answer queries immediately, without waiting on the network. On a
	// genuine first run the DB is empty and nothing is blocked until the
	// background sync below finishes — the trade-off for a non-blocking start.
	if err := filterModule.UpdateFromDb(); err != nil {
		panic(err)
	}
	if err := clients.Sync(); err != nil {
		panic(err)
	}

	// Step 3 of the traffic-dashboard migration: suggest-to-block and
	// domain-inspect now READ allowed-domain data from the unified
	// domain_traffic counter instead of allow_domain_events. The ports are
	// unchanged — only who they read from. Dual-write into allow_domain_events
	// continues (allowWorker below) until Step 7 removes the legacy table.
	trafficAllowAdapter := traffic_db.NewAllowFilterAdapter(trafficRepo)
	suggestModule := suggest_to_block.NewModule(blockRepo, trafficAllowAdapter, sourceRepo, filterModule, suggestRepo, chanLogger)
	domain_inspect_checks.SetAllowLookup(trafficRepo.IsAllowed)

	go clear_events_uc.ClearEvent(blockRepo)
	go allow_clear_events_uc.ClearEvent(allowRepo)
	go suggestModule.Start(context.Background())
	go authBusiness.ClearExpiredSessions()

	// Start the ARP watcher only in LAN mode. Public mode has no LAN to
	// observe; the watcher would just spam ErrUnsupported (or, in a hosted
	// environment with /proc/net/arp present, learn meaningless cloud-VLAN
	// pairs). The watcher exits its own loop on non-Linux platforms.
	if conf.Mode == config.ModeLAN {
		go arpwatcher.Run(context.Background(), chanLogger, arpwatcher.DefaultInterval)
	}

	cacheWithMetric := dns_cache.GetCacheWithMetric()
	metricInstance := dns.CreateMetric()
	allowWorker := allow_domain_use_cases.CreateAllowDomainEventStore(allowRepo, chanLogger, 100)
	blockWorker := block_domain_uc.NewBlockDomainEventStore(blockRepo, chanLogger, 100)
	// Per-device traffic counter (new, unified table). Dual-write alongside the
	// block/allow stores during the staged migration — see TRAFFIC_DASHBOARD_PLAN.md.
	// Capacity bounds DISTINCT aggregation keys held in RAM between flushes, not
	// raw events, so it can be larger than the event stores' batch size.
	trafficWorker := traffic_record_uc.NewTrafficEventStore(trafficRepo, chanLogger, 2000)

	// Reloadable upstream: constructed from env defaults, then re-pointed by the
	// settings hydrate below if a DB override exists. The same instance backs
	// both the hot path and the SWR refresh worker, so a runtime swap repoints
	// both at once.
	resolver := dns.NewReloadableResolver(conf.DoHUpstream, conf.DoHBootstrapIPs...)

	ident := buildIdentifier(conf.Mode)
	dnsServer := dns.CreateServerWithResolver(chanLogger, cacheWithMetric, filterModule.CheckExist, metricInstance, Handlers{
		allowHandler: allowWorker.SendAllowDomainEvent,
		blockHandler: blockWorker.SendBlockDomainEvent,
	}, ident, resolver)
	dnsServer.Traffic = trafficWorker

	// Runtime settings store. Every sink (logger, resolver, cache, server) now
	// exists, so we declare the DB-backed settings, restore the persisted filter
	// toggle, and hydrate effective values into the running process — all before
	// dnsServer.Serve() starts accepting queries.
	settingsModule := settings.NewModule(settingsRepo)
	registerDynamicSettings(settingsModule, dynamicSettingsDeps{
		conf:      conf,
		logr:      chanLogger,
		resolver:  resolver,
		cache:     cacheWithMetric,
		dnsServer: dnsServer,
	})
	filterModule.SetStateSink(filter.PersistHook(settingsRepo, chanLogger))
	if err := filter.RestoreState(settingsRepo, conf); err != nil {
		// Non-fatal: a failed restore leaves the filter at its compiled default
		// (enabled) rather than aborting an otherwise-healthy boot.
		chanLogger.Error(fmt.Errorf("restore filter state: %w", err))
	}
	if err := settingsModule.HydrateAll(); err != nil {
		// Non-fatal: HydrateAll already substituted defaults for any bad rows;
		// this just reports what it skipped.
		chanLogger.Error(fmt.Errorf("settings hydrate: %w", err))
	}

	// Pull the block lists in the background and refresh the filter once done.
	// The DNS server (started below via dnsServer.Serve) does not wait on this.
	// Launched after HydrateAll so the persisted log level is already applied
	// when backgroundSync emits its "started" line — otherwise that INFO line
	// races ahead of hydrate and prints even when the level was raised to WARN,
	// while the matching "finished" line (logged later, post-hydrate) is
	// suppressed, making a healthy sync look stuck.
	go backgroundSync(sourceModule.Sync, filterModule.UpdateFromDb, chanLogger)

	web.CreateServer(web.Handlers{
		Blocked: &blockedWeb.Handlers{
			Repo:          blockRepo,
			Log:           chanLogger,
			RefreshFilter: filterModule.UpdateFromDb,
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
	})

	if err := dnsServer.Serve(); err != nil {
		panic(err)
	}
}
