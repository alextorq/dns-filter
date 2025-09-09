package use_cases

import "github.com/alextorq/dns-filter/metric"

func StartMetric(port string) {
	metric.Serve(port)
}
