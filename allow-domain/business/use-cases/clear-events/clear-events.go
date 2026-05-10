package allow_domain_use_cases_clear_events

import (
	"time"

	"github.com/alextorq/dns-filter/allow-domain/db"
	"github.com/alextorq/dns-filter/periodic"
)

func ClearEvent() {
	const DAYS = 2
	periodic.Run("delete old allow-domain events", 24*time.Hour, func() error {
		return db.DeleteOlderThan(DAYS)
	})
}
