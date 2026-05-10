package check_exist_domain

import (
	"fmt"

	blacklists "github.com/alextorq/dns-filter/blocked-domain"
	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/filter/cache"
	"github.com/alextorq/dns-filter/filter/filter"
	"github.com/alextorq/dns-filter/logger"
)

// TODO after disable or enable domain clear cache
func CheckCacheOrDb(domain string) bool {
	c := cache.GetCache()
	l := logger.GetLogger()
	// Сначала проверяем кэш
	if val, found := c.Get(domain); found {
		l.Debug("get block domain check from cache domain: ", domain, "value: ", found)
		// Возвращаем кэшированный ответ
		return val
	}
	l.Debug("get block domain check from db: ", domain)
	exist, err := blacklists.IsDomainActivelyBlocked(domain)
	if err != nil {
		// Fail-open (don't block) — but skip the cache so a transient DB blip
		// doesn't lock in a "not blocked" verdict for the next ~1500 lookups.
		l.Error(fmt.Errorf("IsDomainActivelyBlocked(%s): %w", domain, err))
		return false
	}
	c.Add(domain, exist)
	return exist
}

func CheckBlock(domain string) bool {
	conf := config.GetConfig()
	if !conf.Enabled.Load() {
		return false
	}
	f := filter.GetFilter()
	if f.DomainExist(domain) {
		return CheckCacheOrDb(domain)
	}
	return false
}
