package main

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"
)

// stubSyncLogger captures backgroundSync's log calls for assertions.
type stubSyncLogger struct {
	mu    sync.Mutex
	infos int
	errs  []error
}

func TestReportServerError_IgnoresExpectedShutdown(t *testing.T) {
	log := &stubSyncLogger{}
	reportServerError("metrics", http.ErrServerClosed, log)
	if len(log.errs) != 0 {
		t.Fatalf("expected shutdown must be quiet, got %v", log.errs)
	}
}

func TestReportServerError_LogsUnexpectedFailure(t *testing.T) {
	log := &stubSyncLogger{}
	boom := errors.New("bind failed")
	reportServerError("metrics", boom, log)
	if len(log.errs) != 1 || !errors.Is(log.errs[0], boom) {
		t.Fatalf("logged errors = %v, want wrapped bind failure", log.errs)
	}
}

func (l *stubSyncLogger) Info(_ ...any) {
	l.mu.Lock()
	l.infos++
	l.mu.Unlock()
}

func (l *stubSyncLogger) Error(err error) {
	l.mu.Lock()
	l.errs = append(l.errs, err)
	l.mu.Unlock()
}

// Happy path: a sync that succeeds on the first attempt triggers the refresh,
// in that order, with no backoff pause and both Info lines logged.
func TestBackgroundSyncRefreshesFilterOnSuccess(t *testing.T) {
	var calls []string
	var sleeps []time.Duration
	log := &stubSyncLogger{}

	runBackgroundSync(
		context.Background(),
		func(context.Context) error { calls = append(calls, "sync"); return nil },
		func() error { calls = append(calls, "refresh"); return nil },
		log,
		func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil },
	)

	if !reflect.DeepEqual(calls, []string{"sync", "refresh"}) {
		t.Fatalf("ожидали sync→refresh, получили %v", calls)
	}
	if len(sleeps) != 0 {
		t.Fatalf("не ожидали пауз при успехе с первой попытки, получили %v", sleeps)
	}
	if len(log.errs) != 0 {
		t.Fatalf("не ожидали ошибок, получили %v", log.errs)
	}
	if log.infos != 2 {
		t.Fatalf("ожидали 2 Info (старт + завершение), получили %d", log.infos)
	}
}

// Negative → recovery: a sync failing twice is retried until it succeeds; the
// backoff doubles between attempts and refresh runs only after sync succeeds.
func TestBackgroundSyncRetriesUntilSyncSucceeds(t *testing.T) {
	var calls []string
	var sleeps []time.Duration
	log := &stubSyncLogger{}
	attempts := 0

	runBackgroundSync(
		context.Background(),
		func(context.Context) error {
			calls = append(calls, "sync")
			attempts++
			if attempts < 3 {
				return errors.New("network down")
			}
			return nil
		},
		func() error { calls = append(calls, "refresh"); return nil },
		log,
		func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil },
	)

	if !reflect.DeepEqual(calls, []string{"sync", "sync", "sync", "refresh"}) {
		t.Fatalf("ожидали 3 попытки sync затем refresh, получили %v", calls)
	}
	if !reflect.DeepEqual(sleeps, []time.Duration{syncRetryBaseDelay, 2 * syncRetryBaseDelay}) {
		t.Fatalf("ожидали растущий backoff [%s %s], получили %v", syncRetryBaseDelay, 2*syncRetryBaseDelay, sleeps)
	}
	if len(log.errs) != 2 {
		t.Fatalf("ожидали 2 ошибки за 2 неудачные попытки, получили %v", log.errs)
	}
	if log.infos != 2 {
		t.Fatalf("ожидали 2 Info (старт + завершение), получили %d", log.infos)
	}
}

// Negative: the exponential backoff must not grow past syncRetryMaxDelay.
func TestBackgroundSyncBackoffIsCapped(t *testing.T) {
	var sleeps []time.Duration
	log := &stubSyncLogger{}
	attempts := 0

	runBackgroundSync(
		context.Background(),
		func(context.Context) error {
			attempts++
			if attempts <= 12 {
				return errors.New("network down")
			}
			return nil
		},
		func() error { return nil },
		log,
		func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil },
	)

	for i, d := range sleeps {
		if d > syncRetryMaxDelay {
			t.Fatalf("пауза #%d = %s превысила максимум %s", i, d, syncRetryMaxDelay)
		}
	}
	if last := sleeps[len(sleeps)-1]; last != syncRetryMaxDelay {
		t.Fatalf("ожидали что backoff упрётся в максимум %s, последняя пауза %s", syncRetryMaxDelay, last)
	}
}

// Negative: a failed refresh after a successful sync is logged, not panicked,
// and no completion Info line follows.
func TestBackgroundSyncLogsRefreshFailure(t *testing.T) {
	var calls []string
	log := &stubSyncLogger{}

	runBackgroundSync(
		context.Background(),
		func(context.Context) error { calls = append(calls, "sync"); return nil },
		func() error { calls = append(calls, "refresh"); return errors.New("db gone") },
		log,
		func(context.Context, time.Duration) error { return nil },
	)

	if !reflect.DeepEqual(calls, []string{"sync", "refresh"}) {
		t.Fatalf("ожидали sync→refresh, получили %v", calls)
	}
	if len(log.errs) != 1 {
		t.Fatalf("ожидали ровно 1 залогированную ошибку от refresh, получили %v", log.errs)
	}
	if log.infos != 1 {
		t.Fatalf("ожидали только стартовый Info (без завершающего), получили %d", log.infos)
	}
}

func TestBackgroundSync_PreCanceledSkipsAllWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	log := &stubSyncLogger{}
	syncCalls, refreshCalls, waitCalls := 0, 0, 0

	runBackgroundSync(
		ctx,
		func(context.Context) error { syncCalls++; return nil },
		func() error { refreshCalls++; return nil },
		log,
		func(context.Context, time.Duration) error { waitCalls++; return nil },
	)

	if syncCalls != 0 || refreshCalls != 0 || waitCalls != 0 {
		t.Fatalf("pre-canceled run did work: sync=%d refresh=%d wait=%d", syncCalls, refreshCalls, waitCalls)
	}
	if log.infos != 0 || len(log.errs) != 0 {
		t.Fatalf("pre-canceled run logged activity: infos=%d errors=%v", log.infos, log.errs)
	}
}

func TestBackgroundSync_CancelDuringBackoffStopsRetriesAndRefresh(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	log := &stubSyncLogger{}
	syncCalls, refreshCalls := 0, 0

	runBackgroundSync(
		ctx,
		func(context.Context) error { syncCalls++; return errors.New("network down") },
		func() error { refreshCalls++; return nil },
		log,
		func(ctx context.Context, _ time.Duration) error {
			cancel()
			return ctx.Err()
		},
	)

	if syncCalls != 1 || refreshCalls != 0 {
		t.Fatalf("cancel during backoff: sync=%d want 1, refresh=%d want 0", syncCalls, refreshCalls)
	}
	if len(log.errs) != 1 || log.infos != 1 {
		t.Fatalf("unexpected logs: infos=%d errors=%v", log.infos, log.errs)
	}
}

func TestBackgroundSync_CancelDuringSyncIsNotLoggedOrRefreshed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	log := &stubSyncLogger{}
	refreshCalls := 0

	runBackgroundSync(
		ctx,
		func(ctx context.Context) error {
			cancel()
			return ctx.Err()
		},
		func() error { refreshCalls++; return nil },
		log,
		func(context.Context, time.Duration) error {
			t.Fatal("wait must not run after cancellation")
			return nil
		},
	)

	if refreshCalls != 0 {
		t.Fatalf("refresh calls = %d, want 0", refreshCalls)
	}
	if len(log.errs) != 0 || log.infos != 1 {
		t.Fatalf("cancellation should be quiet: infos=%d errors=%v", log.infos, log.errs)
	}
}

func TestBackgroundSync_CancelDuringRefreshSkipsCompletionLog(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	log := &stubSyncLogger{}

	runBackgroundSync(
		ctx,
		func(context.Context) error { return nil },
		func() error { cancel(); return nil },
		log,
		func(context.Context, time.Duration) error { t.Fatal("wait must not run"); return nil },
	)

	if log.infos != 1 {
		t.Fatalf("Info calls = %d, want only the startup line", log.infos)
	}
	if len(log.errs) != 0 {
		t.Fatalf("cancellation should not log errors: %v", log.errs)
	}
}

func TestWaitForRetry_PreCanceledReturnsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := waitForRetry(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForRetry error = %v, want context.Canceled", err)
	}
}
