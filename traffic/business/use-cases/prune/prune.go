// Package traffic_use_cases_prune is the daily retention prune over the unified
// per-device traffic counter (domain_traffic). It mirrors the legacy
// blocked-domain/allow-domain clear-events tasks (a 24h periodic loop) but the
// retention window is a runtime-tunable dynamic setting rather than a compile-
// time constant: the prune reads the retention atomic FRESH on every tick, so a
// change made in the UI (which calls SetRetentionDays via the settings Apply
// hook) takes effect on the next prune without a process restart.
//
// "Hot-path/loop readers read the atomic, never the DB" — same convention as the
// other dynamic settings' sinks.
package traffic_use_cases_prune

import (
	"sync/atomic"
	"time"

	"github.com/alextorq/dns-filter/periodic"
)

// retentionDays is the in-memory source of truth the prune loop reads. It is
// written by SetRetentionDays from the settings Apply hook (and at boot by
// HydrateAll), never read from the DB on the loop's path. Seeded to the
// compiled default so a prune before HydrateAll still uses a sane window.
var retentionDays atomic.Int64

func init() { retentionDays.Store(30) }

// SetRetentionDays updates the retention window read by the prune loop. The
// settings descriptor's Apply hook calls this; the value has already been
// validated (1..3650) by the settings module before it reaches here.
func SetRetentionDays(days int) { retentionDays.Store(int64(days)) }

// GetRetentionDays returns the current retention window in days.
func GetRetentionDays() int { return int(retentionDays.Load()) }

// Repo is the output port: pruning rows older than a cutoff.
type Repo interface {
	DeleteOlderThan(cutoff time.Time) error
}

// Run prunes once immediately and then every 24h (matching the legacy
// clear-events cadence). Blocks forever — call from a goroutine.
func Run(repo Repo) {
	periodic.Run("prune old domain_traffic rows", 24*time.Hour, func() error {
		return pruneTaskAt(repo, time.Now())
	})
}

// pruneTaskAt is the unit-testable step: read the CURRENT retention atomic,
// compute the cutoff relative to now, and ask the repo to delete older rows.
// now is injected so tests need no real-time sleeps.
func pruneTaskAt(repo Repo, now time.Time) error {
	cutoff := cutoffForIn(now, GetRetentionDays(), time.Local)
	return repo.DeleteOlderThan(cutoff)
}

// cutoffForIn returns local-midnight-of-now's-day minus days, in loc. Rows are
// bucketed by local-midnight Day (see the record use-case's dayBucket), so the
// cutoff is also a local midnight: DeleteOlderThan uses a strict < so the day
// exactly `days` ago is KEPT and the day before it is the first to be pruned.
func cutoffForIn(now time.Time, days int, loc *time.Location) time.Time {
	n := now.In(loc)
	y, m, d := n.Date()
	midnightToday := time.Date(y, m, d, 0, 0, 0, 0, loc)
	return midnightToday.AddDate(0, 0, -days)
}
