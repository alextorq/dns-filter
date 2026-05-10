package change_filter_dns_records

import (
	"sync"
	"testing"
	"time"

	"github.com/alextorq/dns-filter/config"
)

// Locks in the #28 fix: concurrent toggles must not lose updates.
// With an even total number of toggles, parity must return Enabled to its
// starting value. The previous read-modify-write would drop updates and
// also race under -race.
func TestChangeFilterDnsRecords_ConcurrentTogglesPreserveParity(t *testing.T) {
	conf := config.GetConfig()
	start := conf.Enabled.Load()

	const goroutines = 32
	const togglesPerG = 500
	total := goroutines * togglesPerG // even -> parity returns to start

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range togglesPerG {
				ChangeFilterDnsRecords()
			}
		}()
	}
	wg.Wait()

	if total%2 != 0 {
		t.Fatalf("test invariant: total toggles must be even, got %d", total)
	}
	if got := conf.Enabled.Load(); got != start {
		t.Fatalf("Enabled flipped after even number of toggles: start=%v end=%v", start, got)
	}
}

// Toggling the filter must invalidate any in-flight pause. Otherwise the UI
// would show "Active" while the deadline still suppresses blocking.
func TestChangeFilterDnsRecords_ClearsPause(t *testing.T) {
	conf := config.GetConfig()
	conf.Enabled.Store(true)
	conf.PausedUntilUnix.Store(time.Now().Add(10 * time.Minute).Unix())
	t.Cleanup(func() {
		conf.Enabled.Store(true)
		conf.PausedUntilUnix.Store(0)
	})

	ChangeFilterDnsRecords()

	if got := conf.PausedUntilUnix.Load(); got != 0 {
		t.Fatalf("toggle did not clear pause: got %d, want 0", got)
	}
}
