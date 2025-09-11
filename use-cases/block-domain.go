package use_cases

import (
	"fmt"

	"github.com/alextorq/dns-filter/events"
	"github.com/alextorq/dns-filter/logger"
)

func BlockDomain(domain string) {
	go func() {
		l := logger.GetLogger()
		l.Warn("Заблокирован:", domain)
		err := events.SendEventAboutBlockDomain(domain)
		if err != nil {
			l.Error(fmt.Errorf("ошибка блокировки домена %s: %w", domain, err))
		}
	}()
}
