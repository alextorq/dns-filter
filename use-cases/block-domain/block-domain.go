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
		l.Warn("Заблокирован:", domain)

		record, err := blacklists.GetDomainByName(domain)
		if err != nil {
			l.Error(fmt.Errorf("ошибка получения записи о блокировке %s: %w", domain, err))
			return
		}

		fmt.Println(record)

		err = blocked_domain.CreateBlockDomainEvent(record.ID)
		if err != nil {
			l.Error(fmt.Errorf("ошибка отправки события о блокировки %s: %w", domain, err))
		}
	}()
}
