package use_cases

import "github.com/alextorq/dns-filter/metric"

func StartMetric() {
	metric.Serve()
}
