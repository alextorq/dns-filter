package dns_cache

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

// Manual flush must not bump dns_cache_evictions_total. That counter is the
// signal "your LRU is too small / under pressure"; folding operator-initiated
// flushes into it would make alerts on real eviction pressure impossible to
// tune. This test locks the contract documented in ARCHITECTURE §7.
func TestCacheWithMetrics_ClearDoesNotBumpEvictions(t *testing.T) {
	metrics := newTestMetrics(t)
	c := NewCacheWithMetrics(10, metrics)
	c.Add("a", answerMsg("a.example.com", 60))
	c.Add("b", answerMsg("b.example.com", 60))

	evictionsBefore := testutil.ToFloat64(metrics.cacheEvictions)

	cleared := c.Clear()

	if cleared != 2 {
		t.Fatalf("expected 2 entries cleared, got %d", cleared)
	}
	if got := testutil.ToFloat64(metrics.cacheEvictions); got != evictionsBefore {
		t.Fatalf("manual Clear must not bump evictions: before=%v after=%v", evictionsBefore, got)
	}
	if got := testutil.ToFloat64(metrics.cacheSize); got != 0 {
		t.Fatalf("size gauge must read 0 after Clear, got %v", got)
	}
}

// Subsequent Adds after a manual flush should still drive evictions on real
// LRU pressure — guards against a regression where Clear leaves the cache in
// a state that suppresses the eviction signal.
func TestCacheWithMetrics_EvictionsResumeAfterClear(t *testing.T) {
	metrics := newTestMetrics(t)
	c := NewCacheWithMetrics(2, metrics)
	c.Add("a", answerMsg("a.example.com", 60))
	c.Add("b", answerMsg("b.example.com", 60))
	c.Clear()

	evictionsBefore := testutil.ToFloat64(metrics.cacheEvictions)
	c.Add("c", answerMsg("c.example.com", 60))
	c.Add("d", answerMsg("d.example.com", 60))
	c.Add("e", answerMsg("e.example.com", 60)) // forces one eviction (cap=2)

	if got := testutil.ToFloat64(metrics.cacheEvictions); got != evictionsBefore+1 {
		t.Fatalf("expected one new eviction after refilling past cap, before=%v after=%v",
			evictionsBefore, got)
	}
}

// Hits/misses use the cache-level counters; Lookup wires through to
// State{Fresh,Stale,Expired,Miss}. We don't reassert that here — covered
// by cache_test.go — but we do want to make sure a `Lookup` after Clear
// reports a clean miss (not a sticky Expired from a pre-Clear entry).
func TestCacheWithMetrics_LookupAfterClearIsMiss(t *testing.T) {
	c := NewCacheWithMetrics(10, newTestMetrics(t))
	c.Add("k", answerMsg("k.example.com", 60))
	c.Clear()

	r := c.Lookup("k")
	if r.State != StateMiss {
		t.Fatalf("expected Miss after Clear, got state=%v", r.State)
	}
	if r.Msg != nil {
		t.Fatalf("Miss must not carry a message, got %v", r.Msg)
	}
}

// Concurrent Clear + Add must not deadlock and must leave the gauge in sync
// with the LRU. The exact gauge value after a race is unspecified (Prometheus
// gauges aren't transactional) but it must converge: a final Add settles it.
// Use -race to catch any mutex misuse.
func TestCacheWithMetrics_ClearConcurrentWithAdd(t *testing.T) {
	metrics := newTestMetrics(t)
	c := NewCacheWithMetrics(100, metrics)
	done := make(chan struct{})

	go func() {
		defer close(done)
		for range 200 {
			c.Add("k", answerMsg("k.example.com", 60))
		}
	}()

	for range 50 {
		c.Clear()
	}
	<-done

	// Drive the gauge to a known value to prove the cache is still usable.
	c.Clear()
	c.Add("final", answerMsg("final.example.com", 60))
	if got := testutil.ToFloat64(metrics.cacheSize); got != 1 {
		t.Fatalf("expected size gauge to converge to 1 after final Clear+Add, got %v", got)
	}
}

func TestNewMetrics_IndependentRegistries(t *testing.T) {
	first := newTestMetrics(t)
	second := newTestMetrics(t)
	first.cacheHits.Inc()

	if got := testutil.ToFloat64(first.cacheHits); got != 1 {
		t.Fatalf("first hits = %v, want 1", got)
	}
	if got := testutil.ToFloat64(second.cacheHits); got != 0 {
		t.Fatalf("second hits = %v, want independent zero value", got)
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
