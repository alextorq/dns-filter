package pause_filter

import (
	"sync"
	"testing"
	"time"

	"github.com/alextorq/dns-filter/config"
)

type nopLog struct{}

func (nopLog) Info(args ...any) {}

func freshConf() *config.Config {
	c := &config.Config{}
	c.Enabled.Store(true)
	return c
}

func TestPauseFilter_RejectsInvalidDuration(t *testing.T) {
	conf := freshConf()
	for _, m := range []int{0, 1, 4, 6, 31, -5} {
		if _, err := PauseFilter(conf, nopLog{}, m); err != ErrInvalidDuration {
			t.Fatalf("expected ErrInvalidDuration for %d minutes, got %v", m, err)
		}
	}
	if got := conf.PausedUntilUnix.Load(); got != 0 {
		t.Fatalf("invalid pause should not mutate config, got %d", got)
	}
}

func TestPauseFilter_AcceptsAllowedDurations(t *testing.T) {
	for _, m := range AllowedMinutes {
		conf := freshConf()
		before := time.Now().Unix()
		until, err := PauseFilter(conf, nopLog{}, m)
		after := time.Now().Unix()
		if err != nil {
			t.Fatalf("PauseFilter(%d) failed: %v", m, err)
		}
		minExpected := before + int64(m*60)
		maxExpected := after + int64(m*60)
		if until < minExpected || until > maxExpected {
			t.Fatalf("PauseFilter(%d): until=%d outside [%d,%d]", m, until, minExpected, maxExpected)
		}
		if stored := conf.PausedUntilUnix.Load(); stored != until {
			t.Fatalf("config not updated: got %d, want %d", stored, until)
		}
	}
}

func TestPauseFilter_RejectsWhenFilterDisabled(t *testing.T) {
	conf := freshConf()
	conf.Enabled.Store(false)

	if _, err := PauseFilter(conf, nopLog{}, 5); err != ErrFilterDisabled {
		t.Fatalf("expected ErrFilterDisabled, got %v", err)
	}
	if got := conf.PausedUntilUnix.Load(); got != 0 {
		t.Fatalf("rejected pause must not mutate config, got %d", got)
	}
}

func TestResumeFilter_ClearsPause(t *testing.T) {
	conf := freshConf()
	if _, err := PauseFilter(conf, nopLog{}, 5); err != nil {
		t.Fatalf("PauseFilter failed: %v", err)
	}
	ResumeFilter(conf, nopLog{})
	if got := conf.PausedUntilUnix.Load(); got != 0 {
		t.Fatalf("ResumeFilter did not clear, got %d", got)
	}
	// Resume when not paused must be a no-op.
	ResumeFilter(conf, nopLog{})
}

func TestGetPausedUntil_TreatsExpiredAsZero(t *testing.T) {
	conf := freshConf()
	conf.PausedUntilUnix.Store(time.Now().Unix() - 1)
	if got := GetPausedUntil(conf); got != 0 {
		t.Fatalf("expired pause should return 0, got %d", got)
	}

	future := time.Now().Add(5 * time.Minute).Unix()
	conf.PausedUntilUnix.Store(future)
	if got := GetPausedUntil(conf); got != future {
		t.Fatalf("active pause should return deadline %d, got %d", future, got)
	}
}

// Concurrent pause/resume must not race and must end in a deterministic state
// (last-writer-wins). Run with -race to catch torn reads.
func TestPauseFilter_ConcurrentSafe(t *testing.T) {
	conf := freshConf()
	const goroutines = 16

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range 100 {
				_, _ = PauseFilter(conf, nopLog{}, 5)
			}
		}()
		go func() {
			defer wg.Done()
			for range 100 {
				ResumeFilter(conf, nopLog{})
			}
		}()
	}
	wg.Wait()

	got := conf.PausedUntilUnix.Load()
	if got != 0 && got < time.Now().Unix() {
		t.Fatalf("final PausedUntilUnix is in the past: %d", got)
	}
}

// PauseFilter racing with an external mutator that flips Enabled (the same
// shape as ChangeFilterDnsRecords): no torn reads, no panic, final state is
// always one of {Enabled=true,paused}, {Enabled=true,unpaused},
// {Enabled=false,unpaused}. The "Enabled=false + active pause" combo is
// also acceptable because PauseFilter only writes the deadline AFTER
// observing Enabled=true; a flipper that races AFTER that store cannot un-
// install the pause atomically. This test pins the safety contract under
// -race; the UX inconsistency itself is a separate (open) ticket.
func TestPauseFilter_RaceWithEnabledToggle_NoTornState(t *testing.T) {
	conf := freshConf()
	const goroutines = 16

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range 200 {
				_, _ = PauseFilter(conf, nopLog{}, 5)
			}
		}()
		go func() {
			defer wg.Done()
			for range 200 {
				// Toggle Enabled and wipe any pause — same primitives
				// ChangeFilterDnsRecords uses, without the import cycle.
				old := conf.Enabled.Load()
				conf.Enabled.CompareAndSwap(old, !old)
				conf.PausedUntilUnix.Store(0)
			}
		}()
	}
	wg.Wait()

	until := conf.PausedUntilUnix.Load()
	if until != 0 && until < time.Now().Unix() {
		t.Fatalf("final PausedUntilUnix is in the past: %d", until)
	}
}
