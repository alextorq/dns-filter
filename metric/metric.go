package metric

import (
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type collectorSet struct {
	collectors []prometheus.Collector
}

func (s *collectorSet) Describe(ch chan<- *prometheus.Desc) {
	for _, collector := range s.collectors {
		collector.Describe(ch)
	}
}

func (s *collectorSet) Collect(ch chan<- prometheus.Metric) {
	for _, collector := range s.collectors {
		collector.Collect(ch)
	}
}

// RegisterCollectors registers a component-owned collector bundle in one
// registry operation. A descriptor conflict rejects the whole set, so callers
// never leave behind a partially registered component. Package import no
// longer mutates process state.
func RegisterCollectors(registerer prometheus.Registerer, set ...prometheus.Collector) error {
	if registerer == nil {
		return errors.New("metrics registerer is required")
	}
	if len(set) == 0 {
		return nil
	}
	return registerer.Register(&collectorSet{collectors: set})
}

// RegisterRuntimeCollectors registers process/Go collectors and the logger
// drop counter after main has constructed the logger. Duplicate registration is
// returned to the caller instead of panicking during package initialization.
func RegisterRuntimeCollectors(registerer prometheus.Registerer, droppedCount func() uint64) error {
	return RegisterCollectors(registerer,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "logger_dropped_logs_total",
			Help: "Total log records dropped because the logger channel was full.",
		}, func() float64 { return float64(droppedCount()) }),
	)
}

// NewServer builds a side-effect-free Prometheus HTTP server. A dedicated mux
// prevents metrics routes from mutating http.DefaultServeMux; main owns
// ListenAndServe and Shutdown.
func NewServer(addr string, gatherer prometheus.Gatherer) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}))
	return &http.Server{Addr: addr, Handler: mux}
}
