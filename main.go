// @title           DNS Filter API
// @version         1.0
// @description     HTTP API for the DNS filter: managing block lists, sources, clients, suggestions and runtime config.
// @BasePath        /
package main

import (
	"context"

	allow_domain "github.com/alextorq/dns-filter/allow-domain"
	authBusiness "github.com/alextorq/dns-filter/auth/business"
	blocked_domain "github.com/alextorq/dns-filter/blocked-domain"
	"github.com/alextorq/dns-filter/clients"
	"github.com/alextorq/dns-filter/clients/arpwatcher"
	"github.com/alextorq/dns-filter/clients/identifier"
	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/db/migrate"
	"github.com/alextorq/dns-filter/dns"
	dns_cache "github.com/alextorq/dns-filter/dns-cache"
	"github.com/alextorq/dns-filter/filter"
	"github.com/alextorq/dns-filter/logger"
	"github.com/alextorq/dns-filter/source"
	suggest_to_block "github.com/alextorq/dns-filter/suggest-to-block"
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
	err := source.Sync()
	if err != nil {
		panic(err)
	}

	err = filter.UpdateFilterFromDb()
	if err != nil {
		panic(err)
	}
	if err := clients.Sync(); err != nil {
		panic(err)
	}

	chanLogger := logger.GetLogger()

	go blocked_domain.ClearOldEvent()
	go allow_domain.ClearOldEvent()
	go suggest_to_block.StartCollectSuggest()
	go authBusiness.ClearExpiredSessions()

	// Start the ARP watcher only in LAN mode. Public mode has no LAN to
	// observe; the watcher would just spam ErrUnsupported (or, in a hosted
	// environment with /proc/net/arp present, learn meaningless cloud-VLAN
	// pairs). The watcher exits its own loop on non-Linux platforms.
	if config.GetConfig().Mode == config.ModeLAN {
		go arpwatcher.Run(context.Background(), chanLogger, arpwatcher.DefaultInterval)
	}

	cacheWithMetric := dns_cache.GetCacheWithMetric()
	metricInstance := dns.CreateMetric()
	allowWorker := allow_domain.CreateAllowDomainEventStore(100)
	blockWorker := blocked_domain.CreateBlockDomainEventStore(100)

	ident := buildIdentifier(config.GetConfig().Mode)
	s := dns.CreateServer(chanLogger, cacheWithMetric, filter.CheckExist, metricInstance, Handlers{
		allowHandler: allowWorker.SendAllowDomainEvent,
		blockHandler: blockWorker.SendBlockDomainEvent,
	}, ident)
	web.CreateServer()
	if err := s.Serve(); err != nil {
		panic(err)
	}
}
