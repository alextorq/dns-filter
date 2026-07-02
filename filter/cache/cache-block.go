package cache

import "github.com/alextorq/dns-filter/lru-cache"

type CacheWithMetrics struct {
	inner *lru_cache.LRUCache[bool]
	cap   int
}

func NewCacheWithMetrics(cap int) *CacheWithMetrics {
	return &CacheWithMetrics{
		inner: lru_cache.CreateCache[bool](cap),
		cap:   cap,
	}
}

func (c *CacheWithMetrics) Add(key string, val bool) {
	c.inner.Add(key, val)
}

func (c *CacheWithMetrics) Get(key string) (bool, bool) {
	v, ok := c.inner.Get(key)
	return v, ok
}

func (c *CacheWithMetrics) Clear() {
	c.inner.Clear()
}
