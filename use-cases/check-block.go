package use_cases

import (
	"github.com/alextorq/dns-filter/config"
	blacklists "github.com/alextorq/dns-filter/dns-records"
	"github.com/alextorq/dns-filter/filter"
)

func CheckBlock(domain string) bool {
	conf := config.GetConfig()
	if !conf.Enabled {
		return false
	}
	f := filter.GetFilter()
	if f.DomainExist(domain) {
		return !blacklists.DomainNotExist(domain)
	}
	return false
}
