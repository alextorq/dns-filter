package web

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	traffic_db "github.com/alextorq/dns-filter/traffic/db"
	"github.com/gin-gonic/gin"
)

// dateLayout is the wire format for the from/to query params. Day buckets are
// midnight in the server's local TZ, so a date-only param is the natural grain;
// it is parsed in time.Local to line up with how Day is truncated on write.
const dateLayout = "2006-01-02"

// defaultTopLimit / maxLimit bound the pagination/top-N query params. A zero or
// missing limit falls back to the default; anything over maxLimit is rejected to
// keep an aggregation from scanning the whole table into one response.
const (
	defaultTopLimit = 50
	maxLimit        = 1000
)

// TrafficRepo is the narrow read port the dashboard handlers depend on.
// *traffic/db.Repo satisfies it structurally.
type TrafficRepo interface {
	DeviceSummary(from, to *time.Time) ([]traffic_db.DeviceSummary, error)
	DomainsForDevice(p traffic_db.DeviceDomainsParams) (traffic_db.DomainsResult, error)
	TopDomains(blocked *bool, limit int) ([]traffic_db.DomainCount, error)
}

// Logger is the narrow logging port used by handlers.
type Logger interface {
	Info(args ...any)
	Error(err error)
}

// VendorFunc resolves an OUI vendor display name for a MAC, "" when unknown.
// clients/discovery.LookupVendor satisfies it; injected so the handler stays
// testable without the embedded OUI table.
type VendorFunc func(mac string) string

// Handlers groups the read-only traffic-dashboard endpoints with their
// dependencies. Construct one at the composition root via NewHandlers.
type Handlers struct {
	Repo   TrafficRepo
	Vendor VendorFunc
	Log    Logger
}

// NewHandlers wires the dashboard handlers from the traffic repo, a vendor
// lookup, and a logger.
func NewHandlers(repo TrafficRepo, vendor VendorFunc, log Logger) *Handlers {
	return &Handlers{Repo: repo, Vendor: vendor, Log: log}
}

// parseDateRange reads the optional from/to query params (YYYY-MM-DD). A bad
// value is a 400-worthy error; a missing one yields a nil bound.
func parseDateRange(c *gin.Context) (from, to *time.Time, err error) {
	if v := c.Query("from"); v != "" {
		t, e := time.ParseInLocation(dateLayout, v, time.Local)
		if e != nil {
			return nil, nil, fmt.Errorf("invalid 'from' date %q (want YYYY-MM-DD)", v)
		}
		from = &t
	}
	if v := c.Query("to"); v != "" {
		t, e := time.ParseInLocation(dateLayout, v, time.Local)
		if e != nil {
			return nil, nil, fmt.Errorf("invalid 'to' date %q (want YYYY-MM-DD)", v)
		}
		to = &t
	}
	return from, to, nil
}

// parseBlocked reads the optional blocked query param. Absent → nil (both
// verdicts); a non-boolean value is an error.
func parseBlocked(c *gin.Context) (*bool, error) {
	v := c.Query("blocked")
	if v == "" {
		return nil, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return nil, fmt.Errorf("invalid 'blocked' value %q (want true/false)", v)
	}
	return &b, nil
}

// parseLimit reads an optional positive int query param, falling back to def
// when absent and rejecting non-positive or over-cap values.
func parseLimit(c *gin.Context, def int) (int, error) {
	v := c.Query("limit")
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid 'limit' value %q (want a positive integer)", v)
	}
	if n > maxLimit {
		return 0, fmt.Errorf("'limit' %d exceeds maximum %d", n, maxLimit)
	}
	return n, nil
}

// parseOffset reads an optional non-negative int query param, default 0.
func parseOffset(c *gin.Context) (int, error) {
	v := c.Query("offset")
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid 'offset' value %q (want a non-negative integer)", v)
	}
	return n, nil
}

// GetDevices returns the per-device traffic summaries, enriched with the OUI
// vendor (mac-kind only) and the current IP. An optional from/to date range
// (YYYY-MM-DD) scopes the rollup by day bucket.
// @Summary      Per-device traffic summaries
// @Tags         traffic
// @Produce      json
// @Param        from query string false "Start day, inclusive (YYYY-MM-DD)"
// @Param        to   query string false "End day, inclusive (YYYY-MM-DD)"
// @Success      200 {object} DevicesResponse
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/traffic/devices [get]
func (h *Handlers) GetDevices(c *gin.Context) {
	from, to, err := parseDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	summaries, err := h.Repo.DeviceSummary(from, to)
	if err != nil {
		h.Log.Error(fmt.Errorf("traffic: device summary: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Failed to retrieve devices"})
		return
	}

	devices := make([]DeviceDTO, 0, len(summaries))
	for _, s := range summaries {
		vendor := ""
		if s.ClientKind == "mac" && h.Vendor != nil {
			vendor = h.Vendor(s.ClientValue)
		}
		devices = append(devices, DeviceDTO{
			ClientKind:   s.ClientKind,
			ClientValue:  s.ClientValue,
			CurrentIP:    s.CurrentIP,
			Vendor:       vendor,
			AllowedCount: s.AllowedCount,
			BlockedCount: s.BlockedCount,
			LastSeen:     s.LastSeen.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, DevicesResponse{Devices: devices})
}

// GetDeviceDomains returns the domains a single device queried, with summed
// counts. The device is identified by the kind+value query params (NOT a path
// segment — a MAC value contains colons that are awkward and ambiguous in a
// path). Optional blocked verdict, from/to date range, and limit/offset
// pagination.
// @Summary      Domains for a single device
// @Tags         traffic
// @Produce      json
// @Param        kind    query string true  "Device kind: mac | ip"
// @Param        value   query string true  "Device key: the MAC or IP"
// @Param        blocked query bool   false "Filter by verdict (true=blocked, false=allowed); omit for both"
// @Param        from    query string false "Start day, inclusive (YYYY-MM-DD)"
// @Param        to      query string false "End day, inclusive (YYYY-MM-DD)"
// @Param        limit   query int    false "Max rows (default 50, max 1000)"
// @Param        offset  query int    false "Pagination offset (default 0)"
// @Success      200 {object} DeviceDomainsResponse
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/traffic/devices/domains [get]
func (h *Handlers) GetDeviceDomains(c *gin.Context) {
	kind := c.Query("kind")
	value := c.Query("value")
	if kind == "" || value == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: "both 'kind' and 'value' query params are required"})
		return
	}
	if kind != "mac" && kind != "ip" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: "'kind' must be 'mac' or 'ip'"})
		return
	}

	blocked, err := parseBlocked(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}
	from, to, err := parseDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}
	limit, err := parseLimit(c, defaultTopLimit)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}
	offset, err := parseOffset(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	res, err := h.Repo.DomainsForDevice(traffic_db.DeviceDomainsParams{
		Kind:    kind,
		Value:   value,
		Blocked: blocked,
		From:    from,
		To:      to,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		h.Log.Error(fmt.Errorf("traffic: device domains: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Failed to retrieve device domains"})
		return
	}
	c.JSON(http.StatusOK, DeviceDomainsResponse{Total: res.Total, List: toDomainCountDTOs(res.List)})
}

// GetTopDomains returns the highest-traffic domains across all devices,
// optionally scoped to a verdict, capped at limit.
// @Summary      Top domains by traffic
// @Tags         traffic
// @Produce      json
// @Param        blocked query bool false "Filter by verdict (true=blocked, false=allowed); omit for both"
// @Param        limit   query int  false "Max rows (default 50, max 1000)"
// @Success      200 {object} TopDomainsResponse
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/traffic/top-domains [get]
func (h *Handlers) GetTopDomains(c *gin.Context) {
	blocked, err := parseBlocked(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}
	limit, err := parseLimit(c, defaultTopLimit)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	rows, err := h.Repo.TopDomains(blocked, limit)
	if err != nil {
		h.Log.Error(fmt.Errorf("traffic: top domains: %w", err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Failed to retrieve top domains"})
		return
	}
	c.JSON(http.StatusOK, TopDomainsResponse{List: toDomainCountDTOs(rows)})
}
