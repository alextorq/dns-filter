// @title           DNS Filter API
// @version         1.0
// @description     HTTP API for the DNS filter: managing block lists, sources, clients, suggestions and runtime config.
// @BasePath        /
package main

import (
	"context"

	allow_domain "github.com/alextorq/dns-filter/allow-domain"
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
	"github.com/alextorq/dns-filter/filter"
	filter_cache "github.com/alextorq/dns-filter/filter/cache"
	filter_bloom "github.com/alextorq/dns-filter/filter/filter"
	filterWeb "github.com/alextorq/dns-filter/filter/web"
	"github.com/alextorq/dns-filter/logger"
	"github.com/alextorq/dns-filter/source"
	source_db "github.com/alextorq/dns-filter/source/db"
	sourceWeb "github.com/alextorq/dns-filter/source/web"
	suggest_to_block "github.com/alextorq/dns-filter/suggest-to-block"
	suggest_to_block_db "github.com/alextorq/dns-filter/suggest-to-block/db"
	suggestWeb "github.com/alextorq/dns-filter/suggest-to-block/web"
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

	bloom := filter_bloom.GetFilter()
	cache := filter_cache.GetCache()
	filterModule := filter.NewModule(blockRepo, bloom, cache, conf, chanLogger)

	sourceModule := source.NewModule(sourceRepo, blockRepo, chanLogger)
	sourceModule.Seed()
	if err := sourceModule.Sync(); err != nil {
		panic(err)
	}

	if err := filterModule.UpdateFromDb(); err != nil {
		panic(err)
	}
	if err := clients.Sync(); err != nil {
		panic(err)
	}

	suggestModule := suggest_to_block.NewModule(blockRepo, allowRepo, sourceRepo, filterModule, suggestRepo, chanLogger)

	go clear_events_uc.ClearEvent(blockRepo)
	go allow_domain.ClearOldEvent()
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
	allowWorker := allow_domain.CreateAllowDomainEventStore(100)
	blockWorker := block_domain_uc.NewBlockDomainEventStore(blockRepo, chanLogger, 100)

	ident := buildIdentifier(conf.Mode)
	dnsServer := dns.CreateServer(chanLogger, cacheWithMetric, filterModule.CheckExist, metricInstance, Handlers{
		allowHandler: allowWorker.SendAllowDomainEvent,
		blockHandler: blockWorker.SendBlockDomainEvent,
	}, ident)

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
	})

	if err := dnsServer.Serve(); err != nil {
		panic(err)
	}
}
