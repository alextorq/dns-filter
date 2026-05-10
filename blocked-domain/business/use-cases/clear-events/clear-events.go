package blocked_domain_use_cases_clear_events

import (
	"time"

	"github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/periodic"
)

func ClearEvent() {
	const DAYS = 2
	periodic.Run("delete old blocked-domain events", 24*time.Hour, func() error {
		return db.DeleteOlderThan(DAYS)
	})
}
