package clear_events

import (
	"fmt"
	"time"

	"github.com/alextorq/dns-filter/blocked-domain"
	"github.com/alextorq/dns-filter/logger"
)

func ClearEvent() {
	const DAYS = 14
	l := logger.GetLogger()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// first run
	if err := blocked_domain.DeleteOlderThan(DAYS); err != nil {
		l.Error(fmt.Errorf("error when delete old blocked-domain: %w", err))
	}

	for range ticker.C {
		err := blocked_domain.DeleteOlderThan(DAYS)
		if err != nil {
			l.Error(fmt.Errorf("error when delete old blocked-domain: %w", err))
		}
	}
}
