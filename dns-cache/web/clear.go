package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type CacheFlusher interface {
	Clear() int
}

type Logger interface {
	Info(args ...any)
}

type Handlers struct {
	Cache CacheFlusher
	Log   Logger
}

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
func (h *Handlers) ClearCache(c *gin.Context) {
	cleared := h.Cache.Clear()
	h.Log.Info("DNS response cache cleared via API:", cleared, "entries")
	c.JSON(http.StatusOK, ClearCacheResponse{Cleared: cleared})
}
