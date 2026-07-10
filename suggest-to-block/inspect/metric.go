package inspect

import (
	"github.com/alextorq/dns-filter/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics is the component-owned instrumentation bundle shared by the inspect
// adapter and worker. It is constructed in main and has no package-level state.
type Metrics struct {
	decisions     *prometheus.CounterVec
	rateLimited   prometheus.Counter
	errors        prometheus.Counter
	queueDepth    prometheus.Gauge
	rdapCacheHits prometheus.Counter
}

func NewMetrics(registerer prometheus.Registerer) (*Metrics, error) {
	m := &Metrics{
		decisions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "suggest_inspect_decisions_total",
			Help: "Reputation verdicts produced by the inspect worker, by verdict.",
		}, []string{"verdict"}),
		rateLimited: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "suggest_inspect_rate_limited_total",
			Help: "Inspect runs cut short because a provider returned HTTP 429.",
		}),
		errors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "suggest_inspect_errors_total",
			Help: "Transient (non-rate-limit) inspection failures that were retried.",
		}),
		queueDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "suggest_inspect_queue_depth",
			Help: "Candidates currently sitting in the inspect queue.",
		}),
		rdapCacheHits: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "suggest_inspect_rdap_cache_hits_total",
			Help: "RDAP lookups served from the local registrable-domain cache.",
		}),
	}
	if err := metric.RegisterCollectors(registerer,
		m.decisions,
		m.rateLimited,
		m.errors,
		m.queueDepth,
		m.rdapCacheHits,
	); err != nil {
		return nil, err
	}
	return m, nil
}
