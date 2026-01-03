package blocked_domain

import (
	blocked_domain_use_cases_clear_events "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/clear-events"
	dnsLib "github.com/miekg/dns"
)
import blocked_domain_use_cases_block_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/block-domain"

func ClearOldEvent() {
	blocked_domain_use_cases_clear_events.ClearEvent()
}

func BlockDomain(w dnsLib.ResponseWriter, r *dnsLib.Msg) {
	blocked_domain_use_cases_block_domain.BlockDomain(w, r)
}
