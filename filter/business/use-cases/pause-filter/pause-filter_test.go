package pause_filter

import (
	"sync"
	"testing"
	"time"

	"github.com/alextorq/dns-filter/config"
)

func resetPause(t *testing.T) {
	t.Helper()
	conf := config.GetConfig()
	conf.PausedUntilUnix.Store(0)
	conf.Enabled.Store(true)
}

func TestPauseFilter_RejectsInvalidDuration(t *testing.T) {
	resetPause(t)
	for _, m := range []int{0, 1, 4, 6, 31, -5} {
		if _, err := PauseFilter(m); err != ErrInvalidDuration {
			t.Fatalf("expected ErrInvalidDuration for %d minutes, got %v", m, err)
		}
	}
	if got := config.GetConfig().PausedUntilUnix.Load(); got != 0 {
		t.Fatalf("invalid pause should not mutate config, got %d", got)
	}
}

func TestPauseFilter_AcceptsAllowedDurations(t *testing.T) {
	for _, m := range AllowedMinutes {
		resetPause(t)
		before := time.Now().Unix()
		until, err := PauseFilter(m)
		after := time.Now().Unix()
		if err != nil {
			t.Fatalf("PauseFilter(%d) failed: %v", m, err)
		}
		minExpected := before + int64(m*60)
		maxExpected := after + int64(m*60)
		if until < minExpected || until > maxExpected {
			t.Fatalf("PauseFilter(%d): until=%d outside [%d,%d]", m, until, minExpected, maxExpected)
		}
		if stored := config.GetConfig().PausedUntilUnix.Load(); stored != until {
			t.Fatalf("config not updated: got %d, want %d", stored, until)
		}
	}
}

func TestPauseFilter_RejectsWhenFilterDisabled(t *testing.T) {
	resetPause(t)
	conf := config.GetConfig()
	conf.Enabled.Store(false)
	t.Cleanup(func() { conf.Enabled.Store(true) })

	if _, err := PauseFilter(5); err != ErrFilterDisabled {
		t.Fatalf("expected ErrFilterDisabled, got %v", err)
	}
	if got := conf.PausedUntilUnix.Load(); got != 0 {
		t.Fatalf("rejected pause must not mutate config, got %d", got)
	}
}

func TestResumeFilter_ClearsPause(t *testing.T) {
	resetPause(t)
	if _, err := PauseFilter(5); err != nil {
		t.Fatalf("PauseFilter failed: %v", err)
	}
	ResumeFilter()
	if got := config.GetConfig().PausedUntilUnix.Load(); got != 0 {
		t.Fatalf("ResumeFilter did not clear, got %d", got)
	}
	// Resume when not paused must be a no-op.
	ResumeFilter()
}

func TestGetPausedUntil_TreatsExpiredAsZero(t *testing.T) {
	resetPause(t)
	conf := config.GetConfig()
	conf.PausedUntilUnix.Store(time.Now().Unix() - 1)
	if got := GetPausedUntil(); got != 0 {
		t.Fatalf("expired pause should return 0, got %d", got)
	}

	future := time.Now().Add(5 * time.Minute).Unix()
	conf.PausedUntilUnix.Store(future)
	if got := GetPausedUntil(); got != future {
		t.Fatalf("active pause should return deadline %d, got %d", future, got)
	}
	resetPause(t)
}

// Concurrent pause/resume must not race and must end in a deterministic state
// (last-writer-wins). Run with -race to catch torn reads.
func TestPauseFilter_ConcurrentSafe(t *testing.T) {
	resetPause(t)
	const goroutines = 16

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range 100 {
				_, _ = PauseFilter(5)
			}
		}()
		go func() {
			defer wg.Done()
			for range 100 {
				ResumeFilter()
			}
		}()
	}
	wg.Wait()

	// Final state is one of the two operations; both leave the config in a
	// valid state (0 or a future deadline).
	got := config.GetConfig().PausedUntilUnix.Load()
	if got != 0 && got < time.Now().Unix() {
		t.Fatalf("final PausedUntilUnix is in the past: %d", got)
	}
	resetPause(t)
}
