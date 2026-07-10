package dns

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// swrRefreshTimeout caps how long a background refresh may take. It is
// independent of the client's request context — the client has already been
// served (a Stale answer) and its ctx may have been cancelled, but the
// refresh must still complete so the next request gets a Fresh entry.
const swrRefreshTimeout = 5 * time.Second

// refreshCache is the subset of the cache surface a refresh needs: it
// writes back the freshly fetched response. The same *CacheWithMetrics
// (which implements both Lookup/Add and Get/Add) satisfies it.
type refreshCache interface {
	Add(key string, val *dns.Msg)
}

// refreshWorker fires background refreshes for stale-window hits, bounded
// by a semaphore so a stampede of stale popular domains cannot spawn an
// unbounded number of goroutines.
//
// The semaphore is a counting channel: Refresh tries a non-blocking acquire
// and, if the slot is taken, drops the refresh (the next stale hit will try
// again). singleflight collapses concurrent refreshes for the same key into
// a single upstream call, so the semaphore only needs to bound *distinct*
// in-flight refreshes. The capacity is runtime-tunable via SetConcurrency
// (settings key cache_refresh_concurrency); the default of 32 suits a home
// resolver.
// semaphore bounds the number of in-flight refreshes. It is swapped wholesale
// on a concurrency change (SetConcurrency) rather than resized in place: each
// refresh captures the semaphore it acquired a token from and releases back to
// that same one, so an in-flight refresh started under the old size still
// balances correctly after a resize.
type semaphore struct {
	tokens chan struct{}
}

type refreshWorker struct {
	// limiter is swapped atomically by SetConcurrency. Refresh loads it once
	// per call and both acquires and releases against that snapshot.
	limiter  atomic.Pointer[semaphore]
	cache    refreshCache
	upstream UpstreamResolver
	coord    *upstreamCoordinator
	logger   Logger
	metric   Metric
}

func newRefreshWorker(cache refreshCache, upstream UpstreamResolver, coord *upstreamCoordinator, logger Logger, metric Metric, concurrency int) *refreshWorker {
	w := &refreshWorker{
		cache:    cache,
		upstream: upstream,
		coord:    coord,
		logger:   logger,
		metric:   metric,
	}
	w.SetConcurrency(concurrency)
	return w
}

// SetConcurrency resizes the refresh pool at runtime by swapping in a fresh
// semaphore of capacity n (clamped to >= 1). Subsequent refreshes are bounded
// by the new size; refreshes already in flight keep their captured token.
func (w *refreshWorker) SetConcurrency(n int) {
	if n <= 0 {
		n = 1
	}
	w.limiter.Store(&semaphore{tokens: make(chan struct{}, n)})
}

// Refresh fires an async refresh for (key, question) unless the semaphore
// is saturated, in which case it returns immediately (counted as dropped).
// Safe to call from the hot path — does not block.
func (w *refreshWorker) Refresh(key string, question dns.Question) {
	sem := w.limiter.Load()
	select {
	case sem.tokens <- struct{}{}:
	default:
		w.metric.IncRefresh("dropped")
		return
	}

	go func() {
		defer func() { <-sem.tokens }()

		ctx, cancel := context.WithTimeout(context.Background(), swrRefreshTimeout)
		defer cancel()

		// Coalesce with any concurrent miss/refresh for the same key — a
		// client request that arrives during the refresh will attach to
		// this single in-flight upstream call instead of firing its own.
		_, err := w.coord.Do(key, func() (*dns.Msg, error) {
			resp, err := w.upstream.Exchange(ctx, &dns.Msg{
				MsgHdr:   dns.MsgHdr{RecursionDesired: true},
				Question: []dns.Question{question},
			})
			if err != nil {
				return nil, err
			}
			w.cache.Add(key, resp)
			return resp, nil
		})
		if err != nil {
			w.metric.IncRefresh("error")
			if w.logger != nil {
				w.logger.Debug("SWR refresh failed for", key, ":", err)
			}
			return
		}
		w.metric.IncRefresh("ok")
	}()
}
