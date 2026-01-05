package allow_domain_use_cases

import (
	"fmt"

	"github.com/alextorq/dns-filter/allow-domain/db"
	"github.com/alextorq/dns-filter/logger"
)

func AllowDomain(domain string) {
	go func() {
		l := logger.GetLogger()
		err := db.CreateAllowDomainEvent(domain)
		if err != nil {
			l.Error(fmt.Errorf("ошибка отправки события о разрешенном домене %s: %w", domain, err))
		}
	}()
}
