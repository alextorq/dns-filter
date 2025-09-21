package clear_events

import (
	"fmt"
	"time"

	"github.com/alextorq/dns-filter/events"
	"github.com/alextorq/dns-filter/logger"
)

func ClearEvent() {
	const DAYS = 30
	l := logger.GetLogger()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// first run
	if err := events.DeleteOlderThan(DAYS); err != nil {
		l.Error(fmt.Errorf("error when delete old events: %w", err))
	}

	for range ticker.C {
		err := events.DeleteOlderThan(DAYS)
		if err != nil {
			l.Error(fmt.Errorf("error when delete old events: %w", err))
		}
	}
}
