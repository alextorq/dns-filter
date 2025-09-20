package block_domain

import (
	"github.com/alextorq/dns-filter/events"
)

func SendEventAboutBlockDomain(domain string) error {
	return events.CreateBlockDomainEvent(domain)
}
