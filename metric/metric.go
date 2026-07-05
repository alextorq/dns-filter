package metric

import (
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry remains the shared compatibility registry while feature packages
// still register their collectors during init. Server startup and process-level
// collectors are explicit and owned by main.
var Registry = prometheus.NewRegistry()

// RegisterRuntimeCollectors registers process/Go collectors and the logger
// drop counter after main has constructed the logger. Duplicate registration is
// returned to the caller instead of panicking during package initialization.
func RegisterRuntimeCollectors(registerer prometheus.Registerer, droppedCount func() uint64) error {
	var err error
	for _, collector := range []prometheus.Collector{
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "logger_dropped_logs_total",
			Help: "Total log records dropped because the logger channel was full.",
		}, func() float64 { return float64(droppedCount()) }),
	} {
		if registerErr := registerer.Register(collector); registerErr != nil {
			err = errors.Join(err, registerErr)
		}
	}
	return err
}

// NewServer builds a side-effect-free Prometheus HTTP server. A dedicated mux
// prevents metrics routes from mutating http.DefaultServeMux; main owns
// ListenAndServe and Shutdown.
func NewServer(addr string, gatherer prometheus.Gatherer) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}))
	return &http.Server{Addr: addr, Handler: mux}
}
