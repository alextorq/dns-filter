package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
	"github.com/alextorq/dns-filter/domain-inspect/checks"
	"github.com/alextorq/dns-filter/logger"
	"github.com/gin-gonic/gin"
)

// inspectTimeout caps the whole multi-check fan-out. External services are
// slow and unreliable; we would rather return a partial result with a few
// "timeout" rows than hang the HTTP client.
const inspectTimeout = 8 * time.Second

// checksFactory builds the check catalog the handler runs. It is a function
// (not a captured map) so the underlying registry can hot-swap implementations
// — e.g. when tests want to stub out outbound HTTP. Defaults to the real set.
var checksFactory = checks.Default

// Inspect runs the catalog of domain checks and returns the aggregated result.
// @Summary      Inspect a domain with reputation/diagnostic checks
// @Tags         domain-inspect
// @Produce      json
// @Param        domain query    string true "Domain to inspect (e.g. example.com)"
// @Success      200    {object} domain_inspect.InspectResult
// @Failure      400    {object} ErrorResponse
// @Router       /api/domain/inspect [get]
func Inspect(c *gin.Context) {
	l := logger.GetLogger()

	domain := strings.ToLower(strings.TrimSpace(c.Query("domain")))
	if domain == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: "domain query parameter is required"})
		return
	}
	if strings.ContainsAny(domain, " /\\?#") {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: "domain must be a bare hostname, not a URL"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), inspectTimeout)
	defer cancel()

	res := domain_inspect.Inspect(ctx, domain, checksFactory())
	l.Info(fmt.Sprintf("domain-inspect: %s -> verdict=%s score=%d", domain, res.Summary.Verdict, res.Summary.Score))

	c.JSON(http.StatusOK, res)
}
