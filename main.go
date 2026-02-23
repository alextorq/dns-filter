package main

import (
	allow_domain "github.com/alextorq/dns-filter/allow-domain"
	blocked_domain "github.com/alextorq/dns-filter/blocked-domain"
	"github.com/alextorq/dns-filter/clients"
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

func main() {
	migrate.Migrate()
	err := source.Sync()
	if err != nil {
		panic(err)
	}

	err = filter.UpdateFilterFromDb()
	clients.UpdateClients()

	if err != nil {
		panic(err)
	}

	go blocked_domain.ClearOldEvent()
	go allow_domain.ClearOldEvent()
	go suggest_to_block.StartCollectSuggest()

	chanLogger := logger.GetLogger()
	cacheWithMetric := dns_cache.GetCacheWithMetric()
	metricInstance := dns.CreateMetric()
	allowWorker := allow_domain.CreateAllowDomainEventStore(100)
	blockWorker := blocked_domain.CreateBlockDomainEventStore(100)

	s := dns.CreateServer(chanLogger, cacheWithMetric, filter.CheckExist, metricInstance, Handlers{
		allowHandler: allowWorker.SendAllowDomainEvent,
		blockHandler: blockWorker.SendBlockDomainEvent,
	})
	web.CreateServer()
	s.Serve()
}
