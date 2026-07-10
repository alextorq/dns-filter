package metric

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestRegisterRuntimeCollectors_ExposesDroppedLogs(t *testing.T) {
	registry := prometheus.NewRegistry()
	if err := RegisterRuntimeCollectors(registry, func() uint64 { return 7 }); err != nil {
		t.Fatalf("register runtime collectors: %v", err)
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "logger_dropped_logs_total" {
			continue
		}
		if got := family.Metric[0].Counter.GetValue(); got != 7 {
			t.Fatalf("logger_dropped_logs_total = %v, want 7", got)
		}
		return
	}
	t.Fatal("logger_dropped_logs_total was not registered")
}

func TestRegisterRuntimeCollectors_DuplicateReturnsError(t *testing.T) {
	registry := prometheus.NewRegistry()
	if err := RegisterRuntimeCollectors(registry, func() uint64 { return 0 }); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	if err := RegisterRuntimeCollectors(registry, func() uint64 { return 0 }); err == nil {
		t.Fatal("expected duplicate registration error")
	}
}

func TestRegisterCollectors_RejectsNilRegisterer(t *testing.T) {
	collector := prometheus.NewCounter(prometheus.CounterOpts{Name: "nil_registry_test_total", Help: "test"})
	if err := RegisterCollectors(nil, collector); err == nil {
		t.Fatal("nil registerer must return an error")
	}
}

func TestRegisterCollectors_ConflictDoesNotPartiallyRegisterBundle(t *testing.T) {
	registry := prometheus.NewRegistry()
	conflict := prometheus.NewCounter(prometheus.CounterOpts{Name: "bundle_conflict_total", Help: "existing"})
	if err := registry.Register(conflict); err != nil {
		t.Fatalf("register conflict seed: %v", err)
	}

	unique := prometheus.NewCounter(prometheus.CounterOpts{Name: "bundle_unique_total", Help: "must roll back"})
	duplicate := prometheus.NewCounter(prometheus.CounterOpts{Name: "bundle_conflict_total", Help: "existing"})
	if err := RegisterCollectors(registry, unique, duplicate); err == nil {
		t.Fatal("bundle with a conflicting descriptor must fail")
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() == "bundle_unique_total" {
			t.Fatal("unique collector from rejected bundle was partially registered")
		}
	}
}

func TestNewServer_UsesDedicatedMuxAndConfiguredAddress(t *testing.T) {
	registry := prometheus.NewRegistry()
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_metric", Help: "test"})
	registry.MustRegister(gauge)
	gauge.Set(42)

	srv := NewServer("127.0.0.1:0", registry)
	if srv.Addr != "127.0.0.1:0" {
		t.Fatalf("Addr = %q, want 127.0.0.1:0", srv.Addr)
	}

	metrics := httptest.NewRecorder()
	srv.Handler.ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metrics.Code != http.StatusOK || !strings.Contains(metrics.Body.String(), "test_metric 42") {
		t.Fatalf("metrics response: status=%d body=%q", metrics.Code, metrics.Body.String())
	}

	other := httptest.NewRecorder()
	srv.Handler.ServeHTTP(other, httptest.NewRequest(http.MethodGet, "/other", nil))
	if other.Code != http.StatusNotFound {
		t.Fatalf("dedicated mux must not expose /other, got %d", other.Code)
	}
}
