package dns

import (
	"time"

	"github.com/alextorq/dns-filter/metric"
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	TotalRequests    prometheus.Counter
	ErrorsTotal      *prometheus.CounterVec
	RequestsByType   *prometheus.CounterVec
	RequestsByClient *prometheus.CounterVec
	RequestDuration  prometheus.Histogram
	ResponseSize     prometheus.Histogram
	CacheHits        prometheus.Counter
	CacheMisses      prometheus.Counter
	CacheEvictions   prometheus.Counter
	CacheSize        prometheus.Gauge
}

func CreateMetric() *Metrics {
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
	}

	metric.Registry.MustRegister(
		m.TotalRequests,
		m.ErrorsTotal,
		m.RequestsByType,
		m.RequestsByClient,
		m.RequestDuration,
		m.ResponseSize,
	)

	return m
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
