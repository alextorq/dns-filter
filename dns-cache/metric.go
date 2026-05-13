package dns_cache

import (
	"sync"
	"time"

	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/metric"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	globalCacheWithM *CacheWithMetrics
	onceM            sync.Once
)

var (
	cacheHits = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "dns_cache_hits_total",
		Help: "Total number of cache hits",
	})
	cacheMisses = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "dns_cache_misses_total",
		Help: "Total number of cache misses",
	})
	cacheEvictions = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "dns_cache_evictions_total",
		Help: "Total number of cache evictions",
	})
	cacheSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dns_cache_size",
		Help: "Current number of items in cache",
	})
	// cacheExpired tracks lookups that found an entry whose TTL had
	// elapsed. Split from misses so we can tell "upstream gave us a
	// short TTL" apart from "cold cache" — the former is the signal
	// that this counter is doing its job.
	cacheExpired = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "dns_cache_expired_total",
		Help: "Total number of cache lookups that hit an expired entry",
	})
	// cacheStaleHits tracks lookups inside the stale-window — TTL has
	// elapsed but the entry is still served (with a clamped RR.Ttl) while
	// a background refresh is fired. A non-zero value means SWR is doing
	// its job and clients are seeing instant responses on TTL boundaries.
	cacheStaleHits = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "dns_cache_stale_hits_total",
		Help: "Cache lookups served from the SWR stale-window (past TTL but within staleUntil)",
	})
)

func init() {
	metric.Registry.MustRegister(cacheHits, cacheMisses, cacheEvictions, cacheSize, cacheExpired, cacheStaleHits)
}

type CacheWithMetrics struct {
	inner *Cache
}

func NewCacheWithMetrics(cap int) *CacheWithMetrics {
	return &CacheWithMetrics{
		inner: NewCache(cap),
	}
}

// NewCacheWithMetricsAndSWR builds a metrics-wrapped cache that serves stale
// entries past their TTL for up to staleGrace, returning them with RR.Ttl
// clamped to staleTTL. staleGrace=0 makes Lookup behave exactly like the
// non-SWR cache (no Stale state).
func NewCacheWithMetricsAndSWR(cap int, staleGrace, staleTTL time.Duration) *CacheWithMetrics {
	return &CacheWithMetrics{
		inner: NewCacheWithSWR(cap, staleGrace, staleTTL),
	}
}

func (c *CacheWithMetrics) Add(key string, val *dns.Msg) {
	res := c.inner.Add(key, val)
	if !res.Cached {
		return
	}
	if res.Evicted {
		cacheEvictions.Inc()
	}
	cacheSize.Set(float64(res.Size))
}

func (c *CacheWithMetrics) Get(key string) (*dns.Msg, bool) {
	res := c.inner.Get(key)
	if res.Hit {
		cacheHits.Inc()
		return res.Msg, true
	}
	if res.Expired {
		cacheExpired.Inc()
	}
	cacheMisses.Inc()
	return nil, false
}

// Clear flushes the underlying cache and resyncs the size gauge. Returns the
// number of entries that were evicted so the admin UI / API can report it.
// The eviction counter is intentionally not bumped here: manual flushes are
// an operator action, not LRU pressure, and conflating the two would make
// the dns_cache_evictions_total metric impossible to alert on.
//
// We Set() the gauge to Len() rather than 0 so a concurrent Add that lands
// between our Clear and our gauge write doesn't leave the gauge stuck at 0
// while the LRU has an entry. The gauge can still race with other writers
// (Prometheus gauges aren't transactional), but it converges to the real
// size on the next mutation.
func (c *CacheWithMetrics) Clear() int {
	n := c.inner.Clear()
	cacheSize.Set(float64(c.inner.Len()))
	return n
}

// Len reports the current entry count from the underlying cache.
func (c *CacheWithMetrics) Len() int {
	return c.inner.Len()
}

// Lookup is the SWR-aware accessor: Fresh and Stale both carry a Msg; Stale
// also bumps the dedicated stale-hits counter so we can see when SWR is
// firing on dashboards. Expired and Miss are reported through the
// pre-existing expired/miss counters.
func (c *CacheWithMetrics) Lookup(key string) Lookup {
	r := c.inner.Lookup(key)
	switch r.State {
	case StateFresh:
		cacheHits.Inc()
	case StateStale:
		cacheStaleHits.Inc()
	case StateExpired:
		cacheExpired.Inc()
		cacheMisses.Inc()
	default: // StateMiss
		cacheMisses.Inc()
	}
	return r
}

func GetCacheWithMetric() *CacheWithMetrics {
	onceM.Do(func() {
		conf := config.GetConfig()
		globalCacheWithM = NewCacheWithMetricsAndSWR(1500, conf.CacheStaleGrace, conf.CacheStaleTTL)
	})
	return globalCacheWithM
}
