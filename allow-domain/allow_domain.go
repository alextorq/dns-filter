package allow_domain

import (
	allow_domain_use_cases "github.com/alextorq/dns-filter/allow-domain/business/use-cases"
	allow_domain_use_cases_clear_events "github.com/alextorq/dns-filter/allow-domain/business/use-cases/clear-events"
	dnsLib "github.com/miekg/dns"
)

func AllowDomain(w dnsLib.ResponseWriter, r *dnsLib.Msg) {
	allow_domain_use_cases.AllowDomain(w, r)
}

func ClearOldEvent() {
	allow_domain_use_cases_clear_events.ClearEvent()
}
