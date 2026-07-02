package periodic

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeLogger struct {
	mu     sync.Mutex
	errors []error
}

func newFakeLogger() *fakeLogger {
	return &fakeLogger{}
}

func (l *fakeLogger) Error(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, err)
}

func (l *fakeLogger) firstError() (error, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.errors) == 0 {
		return nil, false
	}
	return l.errors[0], true
}

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	// Long interval so we can be sure the first call is the immediate one,
	// not a tick.
	go Run(ctx, "test-immediate", time.Hour, newFakeLogger(), func() error {
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	go Run(ctx, "test-tick", 10*time.Millisecond, newFakeLogger(), func() error {
		calls.Add(1)
		return nil
	})

	waitFor(t, time.Second, "at least 3 calls", func() bool {
		return calls.Load() >= 3
	})
}

func TestRun_LogsErrorAndContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := newFakeLogger()
	var calls atomic.Int32
	go Run(ctx, "test-error", 10*time.Millisecond, log, func() error {
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

	if err, ok := log.firstError(); ok {
		if !strings.Contains(err.Error(), "test-error: transient failure") {
			t.Fatalf("logged error = %q, want task name and cause", err)
		}
	} else {
		t.Fatal("cleanup error was not logged")
	}
}

func TestRun_ZeroErrorReturnedDoesNotPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Sanity: cleanup may legitimately always return nil — make sure that path
	// does not interact badly with the error-logging branch.
	var calls atomic.Int32
	go Run(ctx, "test-no-error", 5*time.Millisecond, newFakeLogger(), func() error {
		calls.Add(1)
		return nil
	})

	waitFor(t, time.Second, "loop runs at least twice", func() bool {
		return calls.Load() >= 2
	})
}

func TestRun_StopsAfterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	done := make(chan struct{})
	go func() {
		Run(ctx, "test-cancel", 5*time.Millisecond, newFakeLogger(), func() error {
			calls.Add(1)
			return nil
		})
		close(done)
	}()

	waitFor(t, time.Second, "loop runs before cancellation", func() bool {
		return calls.Load() >= 2
	})
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context cancellation")
	}

	afterCancel := calls.Load()
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != afterCancel {
		t.Fatalf("cleanup ran after cancellation: before=%d after=%d", afterCancel, got)
	}
}

func TestRun_PreCanceledContextSkipsCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls atomic.Int32
	Run(ctx, "test-pre-canceled", time.Hour, newFakeLogger(), func() error {
		calls.Add(1)
		return nil
	})

	if got := calls.Load(); got != 0 {
		t.Fatalf("cleanup calls = %d, want 0 for a pre-canceled context", got)
	}
}

func TestRun_CancellationWaitsForInFlightCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	go func() {
		Run(ctx, "test-in-flight", time.Hour, newFakeLogger(), func() error {
			close(started)
			<-release
			return nil
		})
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not start")
	}
	cancel()

	select {
	case <-done:
		t.Fatal("Run returned before the in-flight cleanup completed")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after the in-flight cleanup completed")
	}
}
