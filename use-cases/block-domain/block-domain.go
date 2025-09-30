package block_domain

import (
	"fmt"

	blacklists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/blocked-domain"
	"github.com/alextorq/dns-filter/logger"
	dnsLib "github.com/miekg/dns"
)

func BlockDomain(_ dnsLib.ResponseWriter, r *dnsLib.Msg) {
	go func() {
		l := logger.GetLogger()
		first := r.Question[0]
		domain := first.Name
		//l.Warn("blocked:", domain)

		record, err := blacklists.GetDomainByName(domain)
		if err != nil {
			l.Error(fmt.Errorf("error get domain record from db %s: %w", domain, err))
			return
		}

		err = blocked_domain.CreateBlockDomainEvent(record.ID)
		if err != nil {
			l.Error(fmt.Errorf("error save block domain event %s: %w", domain, err))
		}
	}()
}
