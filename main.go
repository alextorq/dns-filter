package main

import (
	"github.com/alextorq/dns-filter/cache"
	"github.com/alextorq/dns-filter/db/migrate"
	"github.com/alextorq/dns-filter/dns"
	"github.com/alextorq/dns-filter/dns-records/business/use-cases/seed"
	"github.com/alextorq/dns-filter/logger"
	usecases "github.com/alextorq/dns-filter/use-cases"
	"github.com/alextorq/dns-filter/use-cases/allow-domain"
	"github.com/alextorq/dns-filter/use-cases/block-domain"
	clearevents "github.com/alextorq/dns-filter/use-cases/clear-events"
	"github.com/alextorq/dns-filter/web"
	dnsLib "github.com/miekg/dns"
)

type Handlers struct{}

func (h Handlers) Allowed(w dnsLib.ResponseWriter, r *dnsLib.Msg) {
	allow_domain.AllowDomain(w, r)
}

func (h Handlers) Blocked(w dnsLib.ResponseWriter, r *dnsLib.Msg) {
	block_domain.BlockDomain(w, r)
}

func main() {
	migrate.Migrate()
	err := seed.Sync()
	if err != nil {
		panic(err)
	}

	err = usecases.UpdateFilterFromDb()
	if err != nil {
		panic(err)
	}

	go clearevents.ClearEvent()

	chanLogger := logger.GetLogger()
	cacheWithMetric := cache.GetCacheWithMetric()
	metricInstance := dns.CreateMetric()
	handlers := Handlers{}
	s := dns.CreateServer(chanLogger, cacheWithMetric, usecases.CheckBlock, metricInstance, handlers)
	web.CreateSever()
	s.Serve()
}
