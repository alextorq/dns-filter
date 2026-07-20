package inspect

import (
	"context"
	"reflect"
	"time"

	"github.com/alextorq/dns-filter/periodic"
)

// PruneRepo is the output port for retention — *inspect_db.Repo satisfies it
// (DeleteOlderThan prunes both inspect_candidate and rdap_cache).
type PruneRepo interface {
	DeleteOlderThan(cutoff time.Time) error
}

// Clock supplies wall time for the retention cutoff.
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

// StartPrune removes inspect rows last touched more than retentionAge ago, once
// at startup then daily. Cancellation stops future runs after any in-flight DB
// cleanup finishes; call from a goroutine.
//
// retentionAge MUST exceed the inspect cache TTL: an active domain is
// re-inspected every TTL (refreshing CheckedAt), so anything older than a few
// TTLs is a domain that left the traffic set and can be forgotten. The caller
// passes a comfortable multiple of the TTL.
func StartPrune(ctx context.Context, repo PruneRepo, retentionAge time.Duration, clock Clock, log periodic.Logger) {
	if isNilClock(clock) {
		panic("suggest inspect prune: clock is required")
	}
	periodic.Run(ctx, "prune inspect candidates", 24*time.Hour, log, func() error {
		return repo.DeleteOlderThan(clock.Now().Add(-retentionAge))
	})
}
