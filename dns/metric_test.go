package dns

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewMetrics_RegistersInProvidedRegistry(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}

	metrics.IncServeStaleOnError()
	metrics.IncSingleflightCoalesced()
	metrics.IncRefresh("ok")

	want := []string{
		"dns_requests_total",
		"dns_serve_stale_on_error_total",
		"dns_singleflight_coalesced_total",
		"dns_swr_refresh_total",
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	seen := make(map[string]bool, len(families))
	for _, family := range families {
		seen[family.GetName()] = true
	}
	for _, name := range want {
		if !seen[name] {
			t.Errorf("provided registry is missing %s", name)
		}
	}
}

func TestNewMetrics_IndependentRegistries(t *testing.T) {
	first, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("first NewMetrics: %v", err)
	}
	second, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("second NewMetrics: %v", err)
	}

	first.IncRefresh("dropped")
	if got := testutil.ToFloat64(first.refreshTotal.WithLabelValues("dropped")); got != 1 {
		t.Fatalf("first dropped = %v, want 1", got)
	}
	if got := testutil.ToFloat64(second.refreshTotal.WithLabelValues("dropped")); got != 0 {
		t.Fatalf("second dropped = %v, want independent zero value", got)
	}
}

func TestNewMetrics_DuplicateRegistrationReturnsError(t *testing.T) {
	registry := prometheus.NewRegistry()
	if _, err := NewMetrics(registry); err != nil {
		t.Fatalf("first NewMetrics: %v", err)
	}
	if _, err := NewMetrics(registry); err == nil {
		t.Fatal("duplicate metrics in one registry must return an error")
	}
}
