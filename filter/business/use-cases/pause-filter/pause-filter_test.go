package pause_filter

import (
	"sync"
	"testing"
	"time"

	runtime_state "github.com/alextorq/dns-filter/filter/runtime-state"
)

type nopLog struct{}

func (nopLog) Info(args ...any) {}

func freshState() *runtime_state.State { return runtime_state.New(true) }

var pauseTestNow = time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)

func TestPauseFilter_RejectsInvalidDuration(t *testing.T) {
	state := freshState()
	for _, m := range []int{0, 1, 4, 6, 31, -5} {
		if _, err := PauseFilter(state, nopLog{}, m, pauseTestNow); err != ErrInvalidDuration {
			t.Fatalf("expected ErrInvalidDuration for %d minutes, got %v", m, err)
		}
	}
	if got := state.PausedUntil(); got != 0 {
		t.Fatalf("invalid pause should not mutate runtime state, got %d", got)
	}
}

func TestPauseFilter_AcceptsAllowedDurations(t *testing.T) {
	for _, m := range AllowedMinutes {
		state := freshState()
		until, err := PauseFilter(state, nopLog{}, m, pauseTestNow)
		if err != nil {
			t.Fatalf("PauseFilter(%d) failed: %v", m, err)
		}
		expected := pauseTestNow.Add(time.Duration(m) * time.Minute).Unix()
		if until != expected {
			t.Fatalf("PauseFilter(%d): until=%d, want %d", m, until, expected)
		}
		if stored := state.PausedUntil(); stored != until {
			t.Fatalf("runtime state not updated: got %d, want %d", stored, until)
		}
	}
}

func TestPauseFilter_RejectsWhenFilterDisabled(t *testing.T) {
	state := freshState()
	state.SetEnabled(false)

	if _, err := PauseFilter(state, nopLog{}, 5, pauseTestNow); err != ErrFilterDisabled {
		t.Fatalf("expected ErrFilterDisabled, got %v", err)
	}
	if got := state.PausedUntil(); got != 0 {
		t.Fatalf("rejected pause must not mutate runtime state, got %d", got)
	}
}

func TestResumeFilter_ClearsPause(t *testing.T) {
	state := freshState()
	if _, err := PauseFilter(state, nopLog{}, 5, pauseTestNow); err != nil {
		t.Fatalf("PauseFilter failed: %v", err)
	}
	ResumeFilter(state, nopLog{})
	if got := state.PausedUntil(); got != 0 {
		t.Fatalf("ResumeFilter did not clear, got %d", got)
	}
	// Resume when not paused must be a no-op.
	ResumeFilter(state, nopLog{})
}

func TestGetPausedUntil_TreatsExpiredAsZero(t *testing.T) {
	state := freshState()
	state.SetPausedUntil(pauseTestNow.Unix())
	if got := GetPausedUntil(state, pauseTestNow); got != 0 {
		t.Fatalf("pause ending at now should return 0, got %d", got)
	}

	state.SetPausedUntil(pauseTestNow.Unix() - 1)
	if got := GetPausedUntil(state, pauseTestNow); got != 0 {
		t.Fatalf("expired pause should return 0, got %d", got)
	}

	future := pauseTestNow.Add(5 * time.Minute).Unix()
	state.SetPausedUntil(future)
	if got := GetPausedUntil(state, pauseTestNow); got != future {
		t.Fatalf("active pause should return deadline %d, got %d", future, got)
	}
}

// Concurrent pause/resume must not race and must end in a deterministic state
// (last-writer-wins). Run with -race to catch torn reads.
func TestPauseFilter_ConcurrentSafe(t *testing.T) {
	state := freshState()
	const goroutines = 16

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range 100 {
				_, _ = PauseFilter(state, nopLog{}, 5, pauseTestNow)
			}
		}()
		go func() {
			defer wg.Done()
			for range 100 {
				ResumeFilter(state, nopLog{})
			}
		}()
	}
	wg.Wait()

	got := state.PausedUntil()
	if got != 0 && got < pauseTestNow.Unix() {
		t.Fatalf("final pause deadline is in the past: %d", got)
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
	state := freshState()
	const goroutines = 16

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range 200 {
				_, _ = PauseFilter(state, nopLog{}, 5, pauseTestNow)
			}
		}()
		go func() {
			defer wg.Done()
			for range 200 {
				state.ToggleAndResume()
			}
		}()
	}
	wg.Wait()

	until := state.PausedUntil()
	if until != 0 && until < pauseTestNow.Unix() {
		t.Fatalf("final pause deadline is in the past: %d", until)
	}
}
