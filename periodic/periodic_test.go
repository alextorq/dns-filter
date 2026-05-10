package periodic

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// waitFor polls condition until it is true or timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, msg string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}

func TestRun_RunsCleanupImmediately(t *testing.T) {
	var calls atomic.Int32
	// Long interval so we can be sure the first call is the immediate one,
	// not a tick.
	go Run("test-immediate", time.Hour, func() error {
		calls.Add(1)
		return nil
	})

	waitFor(t, time.Second, "first immediate call", func() bool {
		return calls.Load() >= 1
	})

	// Give the loop a moment; it must not tick again within this window.
	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Errorf("expected exactly 1 call before next tick, got %d", got)
	}
}

func TestRun_RunsAgainOnTick(t *testing.T) {
	var calls atomic.Int32
	go Run("test-tick", 10*time.Millisecond, func() error {
		calls.Add(1)
		return nil
	})

	waitFor(t, time.Second, "at least 3 calls", func() bool {
		return calls.Load() >= 3
	})
}

func TestRun_ContinuesAfterError(t *testing.T) {
	var calls atomic.Int32
	go Run("test-error", 10*time.Millisecond, func() error {
		n := calls.Add(1)
		if n%2 == 1 {
			return errors.New("transient failure")
		}
		return nil
	})

	// If errors stopped the loop we'd never see 4 calls.
	waitFor(t, time.Second, "loop survives errors", func() bool {
		return calls.Load() >= 4
	})
}

func TestRun_ZeroErrorReturnedDoesNotPanic(t *testing.T) {
	// Sanity: cleanup may legitimately always return nil — make sure that path
	// does not interact badly with the error-logging branch.
	var calls atomic.Int32
	go Run("test-no-error", 5*time.Millisecond, func() error {
		calls.Add(1)
		return nil
	})

	waitFor(t, time.Second, "loop runs at least twice", func() bool {
		return calls.Load() >= 2
	})
}
