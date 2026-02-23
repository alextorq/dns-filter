package dns_cache

import (
	"sync"

	lru_cache "github.com/alextorq/dns-filter/lru-cache"
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
)

func init() {
	metric.Registry.MustRegister(cacheHits, cacheMisses, cacheEvictions, cacheSize)
}

type CacheWithMetrics struct {
	inner *lru_cache.LRUCache[*dns.Msg]
	cap   int
}

func NewCacheWithMetrics(cap int) *CacheWithMetrics {
	return &CacheWithMetrics{
		inner: lru_cache.CreateCache[*dns.Msg](cap),
		cap:   cap,
	}
}

func (c *CacheWithMetrics) Add(key string, val *dns.Msg) {
	res := c.inner.Add(key, val)

	if res.Evicted {
		cacheEvictions.Inc()
	}
	cacheSize.Set(float64(res.Size))
}

func (c *CacheWithMetrics) Get(key string) (*dns.Msg, bool) {
	v, ok := c.inner.Get(key)
	if ok {
		cacheHits.Inc()
	} else {
		cacheMisses.Inc()
	}
	return v, ok
}

func GetCacheWithMetric() *CacheWithMetrics {
	onceM.Do(func() {
		globalCacheWithM = NewCacheWithMetrics(1500)
	})
	return globalCacheWithM
}
