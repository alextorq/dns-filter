package allow_domain_use_cases_clear_events

import (
	"fmt"
	"time"

	allowdomain "github.com/alextorq/dns-filter/allow-domain"
	"github.com/alextorq/dns-filter/logger"
)

func ClearEvent() {
	const DAYS = 2
	l := logger.GetLogger()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// first run
	if err := allowdomain.DeleteOlderThan(DAYS); err != nil {
		l.Error(fmt.Errorf("error when delete old blocked-domain: %w", err))
	}

	for range ticker.C {
		err := allowdomain.DeleteOlderThan(DAYS)
		if err != nil {
			l.Error(fmt.Errorf("error when delete old blocked-domain: %w", err))
		}
	}
}
