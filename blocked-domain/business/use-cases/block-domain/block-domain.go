package blocked_domain_use_cases_block_domain

import (
	"fmt"

	"github.com/alextorq/dns-filter/blocked-domain/db"
	blacklists "github.com/alextorq/dns-filter/dns-records"
	"github.com/alextorq/dns-filter/logger"
	dnsLib "github.com/miekg/dns"
)

func BlockDomain(_ dnsLib.ResponseWriter, r *dnsLib.Msg) {
	go func() {
		l := logger.GetLogger()
		first := r.Question[0]
		domain := first.Name
		l.Debug("blocked:", domain)

		record, err := blacklists.GetDomainByName(domain)
		if err != nil {
			l.Error(fmt.Errorf("error get domain record from db %s: %w", domain, err))
			return
		}

		err = db.CreateBlockDomainEvent(record.ID)
		if err != nil {
			l.Error(fmt.Errorf("error save block domain event %s: %w", domain, err))
		}
	}()
}
