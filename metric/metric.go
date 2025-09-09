package metric

import (
	"net/http"
	"time"

	"github.com/alextorq/dns-filter/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var l = logger.GetLogger()

var (
	totalRequests = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "dns_requests_total",
			Help: "Total number of DNS requests",
		})

	blockedRequests = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "dns_requests_blocked_total",
			Help: "Total number of blocked DNS requests",
		})

	errorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dns_errors_total",
			Help: "DNS errors by type",
		},
		[]string{"rcode"}, // NXDOMAIN, SERVFAIL и т.п.
	)

	requestsByType = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dns_requests_by_type_total",
			Help: "Requests grouped by DNS query type",
		},
		[]string{"qtype"},
	)

	requestsByClient = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dns_requests_by_client_total",
			Help: "Requests grouped by client IP",
		},
		[]string{"client"},
	)

	requestDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "dns_request_duration_seconds",
			Help:    "Duration of DNS request handling",
			Buckets: prometheus.DefBuckets,
		})

	responseSize = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "dns_response_size_bytes",
			Help:    "Size of DNS responses in bytes",
			Buckets: prometheus.ExponentialBuckets(64, 2, 10), // 64B → ~32KB
		})
)

func init() {
	prometheus.MustRegister(
		totalRequests,
		blockedRequests,
		errorsTotal,
		requestsByType,
		requestsByClient,
		requestDuration,
		responseSize,
	)
}

func Serve(port string) {
	pathListen := ":" + port

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

func HandleDNSRequest(clientIP, qtype, rcode string, respSize int, duration time.Duration, blocked bool) {
	totalRequests.Inc()
	requestsByType.WithLabelValues(qtype).Inc()
	requestsByClient.WithLabelValues(clientIP).Inc()
	requestDuration.Observe(duration.Seconds())
	responseSize.Observe(float64(respSize))

	if blocked {
		blockedRequests.Inc()
	}
	if rcode != "NOERROR" {
		errorsTotal.WithLabelValues(rcode).Inc()
	}
}
