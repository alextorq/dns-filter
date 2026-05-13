package web

import (
	"net/http"

	dns_cache "github.com/alextorq/dns-filter/dns-cache"
	"github.com/alextorq/dns-filter/logger"
	"github.com/gin-gonic/gin"
)

// cacheFlusher is the slice of *CacheWithMetrics this handler needs. Going
// through an interface lets tests inject a fake without touching the global
// singleton, while the production wiring still uses dns_cache.GetCacheWithMetric.
type cacheFlusher interface {
	Clear() int
}

// flusherFactory is overridable from tests so they don't depend on the
// runtime DNS cache singleton (which is shared, package-global state).
var flusherFactory = func() cacheFlusher { return dns_cache.GetCacheWithMetric() }

type ClearCacheResponse struct {
	// Cleared is the number of entries removed by this call. Zero is a
	// legitimate outcome (operator flushed an already-cold cache) and the
	// frontend differentiates it from an error in the toast text.
	Cleared int `json:"cleared"`
}

// ClearCache wipes the upstream DNS response cache. Used by operators to
// force an immediate refresh from upstream — e.g. after rotating a record
// in DNS that has a long TTL.
//
// @Summary      Clear DNS response cache
// @Description  Drops every entry from the in-memory DNS response cache. The next query for each name will be resolved upstream.
// @Tags         dns-cache
// @Produce      json
// @Success      200 {object} ClearCacheResponse
// @Router       /api/dns-cache/clear [post]
func ClearCache(c *gin.Context) {
	cleared := flusherFactory().Clear()
	logger.GetLogger().Info("DNS response cache cleared via API:", cleared, "entries")
	c.JSON(http.StatusOK, ClearCacheResponse{Cleared: cleared})
}
