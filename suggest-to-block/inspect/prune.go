package inspect

import (
	"context"
	"time"

	"github.com/alextorq/dns-filter/periodic"
)

// PruneRepo is the output port for retention — *inspect_db.Repo satisfies it
// (DeleteOlderThan prunes both inspect_candidate and rdap_cache).
type PruneRepo interface {
	DeleteOlderThan(cutoff time.Time) error
}

// StartPrune removes inspect rows last touched more than retentionAge ago, once
// at startup then daily. Cancellation stops future runs after any in-flight DB
// cleanup finishes; call from a goroutine.
//
// retentionAge MUST exceed the inspect cache TTL: an active domain is
// re-inspected every TTL (refreshing CheckedAt), so anything older than a few
// TTLs is a domain that left the traffic set and can be forgotten. The caller
// passes a comfortable multiple of the TTL.
func StartPrune(ctx context.Context, repo PruneRepo, retentionAge time.Duration, log periodic.Logger) {
	periodic.Run(ctx, "prune inspect candidates", 24*time.Hour, log, func() error {
		return repo.DeleteOlderThan(time.Now().Add(-retentionAge))
	})
}
