package blocked_domain_use_cases_clear_events

import (
	"time"

	"github.com/alextorq/dns-filter/periodic"
)

// RetentionDays is how long block events are kept in the DB.
const RetentionDays = 2

// Repo is the output port: pruning old block events.
type Repo interface {
	DeleteEventsOlderThan(days int) error
}

// ClearEvent runs the deletion task once immediately and then every 24h.
// Blocks forever — call from a goroutine.
func ClearEvent(repo Repo) {
	periodic.Run("delete old blocked-domain events", 24*time.Hour, func() error {
		return clearTask(repo)
	})
}

// clearTask is the unit-testable step: one pass over the retention window.
func clearTask(repo Repo) error {
	return repo.DeleteEventsOlderThan(RetentionDays)
}
