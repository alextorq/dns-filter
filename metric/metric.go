package metric

import (
	"net/http"
	"time"

	"github.com/alextorq/dns-filter/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var l = logger.GetLogger()

type Metrics struct {
	Enable           bool
	Port             string
	TotalRequests    prometheus.Counter
	BlockedRequests  prometheus.Counter
	ErrorsTotal      *prometheus.CounterVec
	RequestsByType   *prometheus.CounterVec
	RequestsByClient *prometheus.CounterVec
	RequestDuration  prometheus.Histogram
	ResponseSize     prometheus.Histogram
}

func CreateMetric(enable bool, port string) *Metrics {
	metric := &Metrics{
		Enable: enable,
		Port:   port,
		TotalRequests: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "dns_requests_total",
				Help: "Total number of DNS requests",
			}),
		BlockedRequests: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "dns_requests_blocked_total",
				Help: "Total number of blocked DNS requests",
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

	prometheus.MustRegister(
		metric.TotalRequests,
		metric.BlockedRequests,
		metric.ErrorsTotal,
		metric.RequestsByType,
		metric.RequestsByClient,
		metric.RequestDuration,
		metric.ResponseSize,
	)

	return metric
}

func (m *Metrics) Serve() {
	if m.Enable {
		pathListen := ":" + m.Port

		l.Info("Метрики Prometheus доступны на", pathListen+"/metrics")
		// Запускаем HTTP сервер для метрик в отдельной горутине
		go func() {
			http.Handle("/metrics", promhttp.Handler())
			err := http.ListenAndServe(pathListen, nil)
			if err != nil {
				l.Error(err)
			}
		}()
	}
}

func (m *Metrics) HandleDNSRequest(clientIP, qtype, rcode string, respSize int, duration time.Duration, blocked bool) {
	if m.Enable {
		m.TotalRequests.Inc()
		m.RequestsByType.WithLabelValues(qtype).Inc()
		m.RequestsByClient.WithLabelValues(clientIP).Inc()
		m.RequestDuration.Observe(duration.Seconds())
		m.ResponseSize.Observe(float64(respSize))

		if blocked {
			m.BlockedRequests.Inc()
		}
		if rcode != "NOERROR" {
			m.ErrorsTotal.WithLabelValues(rcode).Inc()
		}
	}
}
