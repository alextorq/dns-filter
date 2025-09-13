package block_domain

import (
	"fmt"

	"github.com/alextorq/dns-filter/logger"
	dnsLib "github.com/miekg/dns"
)

func BlockDomain(w dnsLib.ResponseWriter, r *dnsLib.Msg) {
	go func() {
		first := r.Question[0]
		domain := first.Name
		l := logger.GetLogger()
		l.Warn("Заблокирован:", domain)
		err := SendEventAboutBlockDomain(domain)
		if err != nil {
			l.Error(fmt.Errorf("ошибка отправки события о блокировки %s: %w", domain, err))
		}
	}()
}
