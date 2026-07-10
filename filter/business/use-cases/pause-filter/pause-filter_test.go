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

func TestPauseFilter_RejectsInvalidDuration(t *testing.T) {
	state := freshState()
	for _, m := range []int{0, 1, 4, 6, 31, -5} {
		if _, err := PauseFilter(state, nopLog{}, m); err != ErrInvalidDuration {
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
		before := time.Now().Unix()
		until, err := PauseFilter(state, nopLog{}, m)
		after := time.Now().Unix()
		if err != nil {
			t.Fatalf("PauseFilter(%d) failed: %v", m, err)
		}
		minExpected := before + int64(m*60)
		maxExpected := after + int64(m*60)
		if until < minExpected || until > maxExpected {
			t.Fatalf("PauseFilter(%d): until=%d outside [%d,%d]", m, until, minExpected, maxExpected)
		}
		if stored := state.PausedUntil(); stored != until {
				t.Fatalf("runtime state not updated: got %d, want %d", stored, until)
		}
	}
}

func TestPauseFilter_RejectsWhenFilterDisabled(t *testing.T) {
	state := freshState()
	state.SetEnabled(false)

	if _, err := PauseFilter(state, nopLog{}, 5); err != ErrFilterDisabled {
		t.Fatalf("expected ErrFilterDisabled, got %v", err)
	}
	if got := state.PausedUntil(); got != 0 {
		t.Fatalf("rejected pause must not mutate runtime state, got %d", got)
	}
}

func TestResumeFilter_ClearsPause(t *testing.T) {
	state := freshState()
	if _, err := PauseFilter(state, nopLog{}, 5); err != nil {
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
	state.SetPausedUntil(time.Now().Unix() - 1)
	if got := GetPausedUntil(state); got != 0 {
		t.Fatalf("expired pause should return 0, got %d", got)
	}

	future := time.Now().Add(5 * time.Minute).Unix()
	state.SetPausedUntil(future)
	if got := GetPausedUntil(state); got != future {
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
				_, _ = PauseFilter(state, nopLog{}, 5)
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
	if got != 0 && got < time.Now().Unix() {
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
				_, _ = PauseFilter(state, nopLog{}, 5)
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
	if until != 0 && until < time.Now().Unix() {
		t.Fatalf("final pause deadline is in the past: %d", until)
	}
}
