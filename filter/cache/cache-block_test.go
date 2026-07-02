package cache

import "testing"

func TestNewCacheWithMetrics_ReturnsIndependentInstances(t *testing.T) {
	first := NewCacheWithMetrics(2)
	second := NewCacheWithMetrics(2)

	first.Add("blocked.example.", true)

	if value, ok := first.Get("blocked.example."); !ok || !value {
		t.Fatal("first cache should contain the inserted verdict")
	}
	if _, ok := second.Get("blocked.example."); ok {
		t.Fatal("verdict added to one cache leaked into another instance")
	}
}
