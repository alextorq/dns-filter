package dns

import (
	"context"
	"time"

	"github.com/alextorq/dns-filter/metric"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

// swrRefreshTimeout caps how long a background refresh may take. It is
// independent of the client's request context — the client has already been
// served (a Stale answer) and its ctx may have been cancelled, but the
// refresh must still complete so the next request gets a Fresh entry.
const swrRefreshTimeout = 5 * time.Second

var refreshTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "dns_swr_refresh_total",
		Help: "Background SWR refresh attempts, broken down by outcome (ok, error, dropped)",
	},
	[]string{"result"},
)

func init() {
	metric.Registry.MustRegister(refreshTotal)
	// Pre-touch every label so all three series are visible on dashboards
	// from process start, even before the first refresh fires. Without this
	// an alert like "absent(dns_swr_refresh_total{result='error'})" would
	// page on a perfectly healthy fresh-boot resolver.
	for _, result := range []string{"ok", "error", "dropped"} {
		refreshTotal.WithLabelValues(result)
	}
}

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
// in-flight refreshes — 32 is plenty for a home resolver.
type refreshWorker struct {
	sem      chan struct{}
	cache    refreshCache
	upstream UpstreamResolver
	coord    *upstreamCoordinator
	logger   Logger
}

func newRefreshWorker(cache refreshCache, upstream UpstreamResolver, coord *upstreamCoordinator, logger Logger, concurrency int) *refreshWorker {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &refreshWorker{
		sem:      make(chan struct{}, concurrency),
		cache:    cache,
		upstream: upstream,
		coord:    coord,
		logger:   logger,
	}
}

// Refresh fires an async refresh for (key, question) unless the semaphore
// is saturated, in which case it returns immediately (counted as dropped).
// Safe to call from the hot path — does not block.
func (w *refreshWorker) Refresh(key string, question dns.Question) {
	select {
	case w.sem <- struct{}{}:
	default:
		refreshTotal.WithLabelValues("dropped").Inc()
		return
	}

	go func() {
		defer func() { <-w.sem }()

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
			refreshTotal.WithLabelValues("error").Inc()
			if w.logger != nil {
				w.logger.Debug("SWR refresh failed for", key, ":", err)
			}
			return
		}
		refreshTotal.WithLabelValues("ok").Inc()
	}()
}
