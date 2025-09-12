package main

import (
	"github.com/alextorq/dns-filter/cache"
	"github.com/alextorq/dns-filter/db/migrate"
	"github.com/alextorq/dns-filter/dns"
	"github.com/alextorq/dns-filter/logger"
	usecases "github.com/alextorq/dns-filter/use-cases"
)

func main() {
	migrate.Migrate()
	err := usecases.GetFromDb()
	if err != nil {
		panic(err)
	}
	chanLogger := logger.GetLogger()
	cacheWithMetric := cache.GetCacheWithMetric()
	metricInstance := dns.CreateMetric()
	s := dns.CreateServer(chanLogger, cacheWithMetric, usecases.CheckBlock, metricInstance)
	s.Serve()
}
