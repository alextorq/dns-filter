package use_cases

import (
	blacklists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/filter"
)

func CheckBlock(domain string) bool {
	f := filter.GetFilter()
	if f.DomainExist(domain) {
		return !blacklists.DomainNotExist(domain)
	}
	return false
}
