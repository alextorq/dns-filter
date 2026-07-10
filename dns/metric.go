package dns

import (
	"time"

	"github.com/alextorq/dns-filter/metric"
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	TotalRequests         prometheus.Counter
	ErrorsTotal           *prometheus.CounterVec
	RequestsByType        *prometheus.CounterVec
	RequestsByClient      *prometheus.CounterVec
	RequestDuration       prometheus.Histogram
	ResponseSize          prometheus.Histogram
	serveStaleOnError     prometheus.Counter
	singleflightCoalesced prometheus.Counter
	refreshTotal          *prometheus.CounterVec
}

// NewMetrics constructs and registers the complete DNS instrumentation bundle
// in the caller-owned registry. It returns registration errors instead of
// panicking during package initialization.
func NewMetrics(registerer prometheus.Registerer) (*Metrics, error) {
	m := &Metrics{
		TotalRequests: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "dns_requests_total",
				Help: "Total number of DNS requests",
			}),
		ErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "dns_errors_total",
				Help: "DNS errors by type",
			},
			[]string{"rcode"}, // NXDOMAIN, SERVFAIL и т.п.
		),
		RequestsByType: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "dns_requests_by_type_total",
				Help: "Requests grouped by DNS query type",
			}, []string{"qtype"}),
		RequestsByClient: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "dns_requests_by_client_total",
				Help: "Requests grouped by client IP",
			}, []string{"client"}),
		RequestDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "dns_request_duration_seconds",
				Help:    "Duration of DNS request handling",
				Buckets: prometheus.DefBuckets,
			}),
		ResponseSize: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "dns_response_size_bytes",
				Help:    "Size of DNS responses in bytes",
				Buckets: prometheus.ExponentialBuckets(64, 2, 10), // 64B → ~32KB
			}),
		serveStaleOnError: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dns_serve_stale_on_error_total",
			Help: "DNS responses served from the stale-window because upstream returned an error (RFC 8767)",
		}),
		singleflightCoalesced: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dns_singleflight_coalesced_total",
			Help: "DNS queries that received a singleflight-shared upstream result (counts every caller in a coalesced group, including the owner; saved upstream calls = value minus number of groups)",
		}),
		refreshTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "dns_swr_refresh_total",
			Help: "Background SWR refresh attempts, broken down by outcome (ok, error, dropped)",
		}, []string{"result"}),
	}

	// Keep every result series visible from process start so absence-based alerts
	// do not fire on a healthy fresh boot.
	for _, result := range []string{"ok", "error", "dropped"} {
		m.refreshTotal.WithLabelValues(result)
	}

	if err := metric.RegisterCollectors(registerer,
		m.TotalRequests,
		m.ErrorsTotal,
		m.RequestsByType,
		m.RequestsByClient,
		m.RequestDuration,
		m.ResponseSize,
		m.serveStaleOnError,
		m.singleflightCoalesced,
		m.refreshTotal,
	); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Metrics) HandleDNSRequest(clientIP, qtype, rcode string, respSize int, duration time.Duration) {
	m.TotalRequests.Inc()
	m.RequestsByType.WithLabelValues(qtype).Inc()
	m.RequestsByClient.WithLabelValues(clientIP).Inc()
	m.RequestDuration.Observe(duration.Seconds())
	m.ResponseSize.Observe(float64(respSize))

	if rcode != "NOERROR" {
		m.ErrorsTotal.WithLabelValues(rcode).Inc()
	}
}

func (m *Metrics) IncServeStaleOnError() { m.serveStaleOnError.Inc() }

func (m *Metrics) IncSingleflightCoalesced() { m.singleflightCoalesced.Inc() }

func (m *Metrics) IncRefresh(result string) { m.refreshTotal.WithLabelValues(result).Inc() }
