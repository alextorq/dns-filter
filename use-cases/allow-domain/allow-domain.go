package allow_domain

import (
	"fmt"

	"github.com/alextorq/dns-filter/logger"
	dnsLib "github.com/miekg/dns"
)

func AllowDomain(w dnsLib.ResponseWriter, r *dnsLib.Msg) {
	go func() {
		l := logger.GetLogger()
		first := r.Question[0]
		domain := first.Name
		err := SendEventAboutAllowDomain(domain)
		if err != nil {
			l.Error(fmt.Errorf("ошибка отправки события о разрешенном домене %s: %w", domain, err))
		}
	}()
}
