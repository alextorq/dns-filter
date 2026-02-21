package main

import (
	allow_domain "github.com/alextorq/dns-filter/allow-domain"
	blocked_domain "github.com/alextorq/dns-filter/blocked-domain"
	"github.com/alextorq/dns-filter/cache"
	"github.com/alextorq/dns-filter/clients"
	"github.com/alextorq/dns-filter/db/migrate"
	"github.com/alextorq/dns-filter/dns"
	"github.com/alextorq/dns-filter/filter"
	"github.com/alextorq/dns-filter/logger"
	"github.com/alextorq/dns-filter/source"
	suggest_to_block "github.com/alextorq/dns-filter/suggest-to-block"
	usecases "github.com/alextorq/dns-filter/use-cases"
	"github.com/alextorq/dns-filter/web"
	dnsLib "github.com/miekg/dns"
)

type Handlers struct {
	allowHandler func(domain string)
}

func (h Handlers) Allowed(_ dnsLib.ResponseWriter, r *dnsLib.Msg) {
	first := r.Question[0]
	domain := first.Name
	h.allowHandler(domain)
}

func (h Handlers) Blocked(w dnsLib.ResponseWriter, r *dnsLib.Msg) {
	blocked_domain.BlockDomain(w, r)
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
	blocked_domain.StartEventWorker()

	chanLogger := logger.GetLogger()
	cacheWithMetric := cache.GetCacheWithMetric()
	metricInstance := dns.CreateMetric()
	allowWorker := allow_domain.CreateAllowDomainEventStore(100)

	s := dns.CreateServer(chanLogger, cacheWithMetric, usecases.CheckBlock, metricInstance, Handlers{
		allowHandler: allowWorker.SendAllowDomainEvent,
	})
	web.CreateServer()
	s.Serve()
}
