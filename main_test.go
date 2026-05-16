package main

import (
	"errors"
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
		func() error { calls = append(calls, "sync"); return nil },
		func() error { calls = append(calls, "refresh"); return nil },
		log,
		func(d time.Duration) { sleeps = append(sleeps, d) },
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
		func() error {
			calls = append(calls, "sync")
			attempts++
			if attempts < 3 {
				return errors.New("network down")
			}
			return nil
		},
		func() error { calls = append(calls, "refresh"); return nil },
		log,
		func(d time.Duration) { sleeps = append(sleeps, d) },
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
		func() error {
			attempts++
			if attempts <= 12 {
				return errors.New("network down")
			}
			return nil
		},
		func() error { return nil },
		log,
		func(d time.Duration) { sleeps = append(sleeps, d) },
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
		func() error { calls = append(calls, "sync"); return nil },
		func() error { calls = append(calls, "refresh"); return errors.New("db gone") },
		log,
		func(time.Duration) {},
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
