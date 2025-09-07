package use_cases

import "dns-filter/metric"

func StartMetric() {
	metric.Serve()
}
