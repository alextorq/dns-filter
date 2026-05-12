package dns_cache

import (
	"sync"

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
)

func init() {
	metric.Registry.MustRegister(cacheHits, cacheMisses, cacheEvictions, cacheSize, cacheExpired)
}

type CacheWithMetrics struct {
	inner *Cache
}

func NewCacheWithMetrics(cap int) *CacheWithMetrics {
	return &CacheWithMetrics{
		inner: NewCache(cap),
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

func GetCacheWithMetric() *CacheWithMetrics {
	onceM.Do(func() {
		globalCacheWithM = NewCacheWithMetrics(1500)
	})
	return globalCacheWithM
}
