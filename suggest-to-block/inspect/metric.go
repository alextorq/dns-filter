package inspect

import (
	"github.com/alextorq/dns-filter/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// Prometheus instrumentation for the reputation worker. Registered in init()
// against the shared registry (same pattern as dns-cache/metric.go). Without
// these an operator cannot see that, say, VirusTotal is rate-limiting most
// inspections.
var (
	inspectDecisions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "suggest_inspect_decisions_total",
		Help: "Reputation verdicts produced by the inspect worker, by verdict.",
	}, []string{"verdict"})

	inspectRateLimited = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "suggest_inspect_rate_limited_total",
		Help: "Inspect runs cut short because a provider returned HTTP 429.",
	})

	inspectErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "suggest_inspect_errors_total",
		Help: "Transient (non-rate-limit) inspection failures that were retried.",
	})

	inspectQueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "suggest_inspect_queue_depth",
		Help: "Candidates currently sitting in the inspect queue.",
	})

	inspectRDAPCacheHits = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "suggest_inspect_rdap_cache_hits_total",
		Help: "RDAP lookups served from the registrable cache instead of the network.",
	})
)

func init() {
	metric.Registry.MustRegister(
		inspectDecisions,
		inspectRateLimited,
		inspectErrors,
		inspectQueueDepth,
		inspectRDAPCacheHits,
	)
}
