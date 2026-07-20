// Package traffic_use_cases_prune is the daily retention prune over the unified
// per-device traffic counter (domain_traffic). It mirrors the legacy
// blocked-domain/allow-domain clear-events tasks (a 24h periodic loop) but the
// retention window is a runtime-tunable dynamic setting rather than a compile-
// time constant: the prune reads its injected retention state FRESH on every tick, so a
// change made in the UI (which calls RetentionState.Set via the settings Apply
// hook) takes effect on the next prune without a process restart.
//
// "Hot-path/loop readers read the atomic, never the DB" — same convention as the
// other dynamic settings' sinks.
package traffic_use_cases_prune

import (
	"context"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/alextorq/dns-filter/periodic"
)

// RetentionState is the in-memory source of truth the prune loop reads. It is
// constructed in the composition root, written by the settings Apply hook (and
// at boot by HydrateAll), and never read from the DB on the loop's path.
//
// Seeded to retentionUnset (0): until HydrateAll has applied the effective
// window, the prune MUST NOT run. A seeded guess (the old code used 30) is only
// "sane" when it is >= the operator's real window — a larger DB override (e.g.
// 365) would have day-buckets between 30 and 365 days old hard-deleted by a
// prune that fired before hydrate. main launches the loop only after HydrateAll,
// and HydrateAll always applies a value (DB override or compiled default) via
// RetentionState.Set, so the loop is armed before its first real prune; this
// sentinel is the belt-and-suspenders guard against a future reordering.
type RetentionState struct {
	days atomic.Int64
}

// retentionUnset marks the pre-hydrate state. It is outside the validated range
// (1..3650 enforced by the settings module), so a real configured value is
// always > 0.
const retentionUnset = 0

// NewRetentionState returns an unconfigured state. The zero value is also ready
// to use and carries the same safe retentionUnset sentinel.
func NewRetentionState() *RetentionState { return &RetentionState{} }

// Set updates the retention window read by the prune loop. The
// settings descriptor's Apply hook calls this; the value has already been
// validated (1..3650) by the settings module before it reaches here.
func (s *RetentionState) Set(days int) { s.days.Store(int64(days)) }

// Days returns the current retention window in days.
func (s *RetentionState) Days() int { return int(s.days.Load()) }

// Repo is the output port: pruning rows older than a cutoff.
type Repo interface {
	DeleteOlderThan(cutoff time.Time) error
}

// Clock supplies wall time for daily retention cutoffs.
type Clock interface {
	Now() time.Time
}

func isNilClock(clock Clock) bool {
	if clock == nil {
		return true
	}
	v := reflect.ValueOf(clock)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// Run prunes once immediately and then every 24h (matching the legacy
// clear-events cadence). Cancellation stops future runs after any in-flight DB
// cleanup finishes; call from a goroutine.
func Run(ctx context.Context, repo Repo, state *RetentionState, clock Clock, log periodic.Logger) {
	if isNilClock(clock) {
		panic("traffic prune: clock is required")
	}
	periodic.Run(ctx, "prune old domain_traffic rows", 24*time.Hour, log, func() error {
		return pruneTaskAt(repo, state, clock.Now())
	})
}

// pruneTaskAt is the unit-testable step: read the CURRENT injected retention state,
// compute the cutoff relative to now, and ask the repo to delete older rows.
// now is injected so tests need no real-time sleeps.
func pruneTaskAt(repo Repo, state *RetentionState, now time.Time) error {
	days := state.Days()
	if days <= retentionUnset {
		// Not configured yet (a prune fired before HydrateAll). Skip rather than
		// delete with an unconfigured/guessed window — see RetentionState.
		return nil
	}
	cutoff := cutoffForIn(now, days, time.Local)
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
