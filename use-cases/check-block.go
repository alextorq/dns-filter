package use_cases

import (
	blacklists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/config"
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
