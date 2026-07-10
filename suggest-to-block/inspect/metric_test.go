package inspect

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func newTestMetrics(t *testing.T) *Metrics {
	t.Helper()
	metrics, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	return metrics
}

func TestNewMetrics_IndependentRegistries(t *testing.T) {
	first := newTestMetrics(t)
	second := newTestMetrics(t)
	first.decisions.WithLabelValues("clean").Inc()

	if got := testutil.ToFloat64(first.decisions.WithLabelValues("clean")); got != 1 {
		t.Fatalf("first clean decisions = %v, want 1", got)
	}
	if got := testutil.ToFloat64(second.decisions.WithLabelValues("clean")); got != 0 {
		t.Fatalf("second clean decisions = %v, want independent zero value", got)
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
