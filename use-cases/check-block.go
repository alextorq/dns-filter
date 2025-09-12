package use_cases

import (
	black_lists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/filter"
)

func CheckBlock(domain string) bool {
	f := filter.GetFilter()
	if f.DomainExist(domain) {
		return !black_lists.DomainNotExist(domain)
	}
	return false
}
