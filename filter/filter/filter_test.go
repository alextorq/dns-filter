package filter

import (
	"testing"
)

// Locks in #29: an empty input must not collapse the bloom to zero
// capacity (which the previous code did via NewWithEstimates(0, …)) and
// subsequent lookups must safely return false.
func TestUpdateFilter_EmptyInputDoesNotPanic(t *testing.T) {
	f := &Filter{}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("UpdateFilter([]) panicked: %v", r)
		}
	}()

	f.UpdateFilter(nil)
	if f.DomainExist("anything.example.") {
		t.Fatal("empty bloom should not report any domain as present")
	}
}

func TestUpdateFilter_StoresDomain(t *testing.T) {
	f := &Filter{}
	f.UpdateFilter([]string{"blocked.example."})

	if !f.DomainExist("blocked.example.") {
		t.Fatal("freshly added domain should be reported as present")
	}
}

// A small input must not produce a high false-positive rate. Before the
// floor was introduced, a 1-item input would size the bloom for 1 element
// and inflate FP for any subsequent membership tests.
func TestUpdateFilter_FloorKeepsFalsePositiveRateLow(t *testing.T) {
	f := &Filter{}
	f.UpdateFilter([]string{"blocked.example."})

	const probes = 1000
	hits := 0
	for i := range probes {
		if f.DomainExist(string(rune('a'+i%26)) + ".not-blocked.example.") {
			hits++
		}
	}
	// With the 10M floor at 0.1% FP, hits over 1k random misses should be ~0–1.
	// Allow generous slack so the test does not flake across hash-seed runs.
	if hits > 5 {
		t.Fatalf("unexpectedly high false-positive count: %d/%d (bloom likely undersized)", hits, probes)
	}
}
