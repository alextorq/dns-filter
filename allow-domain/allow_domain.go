package allow_domain

import (
	allow_domain_use_cases "github.com/alextorq/dns-filter/allow-domain/business/use-cases"
	allow_domain_use_cases_clear_events "github.com/alextorq/dns-filter/allow-domain/business/use-cases/clear-events"
	allow_domain_db "github.com/alextorq/dns-filter/allow-domain/db"
	dnsLib "github.com/miekg/dns"
)

func AllowDomain(_ dnsLib.ResponseWriter, r *dnsLib.Msg) {
	first := r.Question[0]
	domain := first.Name
	allow_domain_use_cases.AllowDomain(domain)
}

func ClearOldEvent() {
	allow_domain_use_cases_clear_events.ClearEvent()
}

func GetAllActiveFilters() ([]string, error) {
	return allow_domain_db.GetAllActiveFilters()
}
