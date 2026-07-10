package sourcesync

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncerFunc func(context.Context) error

func (f syncerFunc) Sync(ctx context.Context) error { return f(ctx) }

type refresherFunc func() error

func (f refresherFunc) UpdateFromDb() error { return f() }

type stubLogger struct {
	mu    sync.Mutex
	infos int
	errs  []error
}

func (l *stubLogger) Info(_ ...any) {
	l.mu.Lock()
	l.infos++
	l.mu.Unlock()
}

func (l *stubLogger) Error(err error) {
	l.mu.Lock()
	l.errs = append(l.errs, err)
	l.mu.Unlock()
}

func newTestJob(t *testing.T, syncer Syncer, refresher FilterRefresher, log Logger, wait waitFunc) *Job {
	t.Helper()
	job, err := New(syncer, refresher, log)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	job.wait = wait
	return job
}

func TestJob_RunRefreshesFilterAfterSuccessfulSync(t *testing.T) {
	var calls []string
	var waits []time.Duration
	log := &stubLogger{}
	job := newTestJob(t,
		syncerFunc(func(context.Context) error { calls = append(calls, "sync"); return nil }),
		refresherFunc(func() error { calls = append(calls, "refresh"); return nil }),
		log,
		func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil },
	)

	job.Run(context.Background())

	if !reflect.DeepEqual(calls, []string{"sync", "refresh"}) {
		t.Fatalf("calls = %v, want sync then refresh", calls)
	}
	if len(waits) != 0 {
		t.Fatalf("waits = %v, want none", waits)
	}
	if len(log.errs) != 0 || log.infos != 2 {
		t.Fatalf("logs: infos=%d errors=%v, want two infos and no errors", log.infos, log.errs)
	}
}

func TestJob_RunRetriesWithCappedExponentialBackoff(t *testing.T) {
	var calls []string
	var waits []time.Duration
	attempts := 0
	log := &stubLogger{}
	job := newTestJob(t,
		syncerFunc(func(context.Context) error {
			calls = append(calls, "sync")
			attempts++
			if attempts <= 12 {
				return errors.New("network down")
			}
			return nil
		}),
		refresherFunc(func() error { calls = append(calls, "refresh"); return nil }),
		log,
		func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil },
	)

	job.Run(context.Background())

	if len(calls) != 14 || calls[len(calls)-1] != "refresh" {
		t.Fatalf("calls = %v, want 13 sync attempts then refresh", calls)
	}
	if len(waits) != 12 {
		t.Fatalf("wait count = %d, want 12", len(waits))
	}
	if waits[0] != retryBaseDelay || waits[1] != 2*retryBaseDelay {
		t.Fatalf("initial waits = %v, want %s then %s", waits[:2], retryBaseDelay, 2*retryBaseDelay)
	}
	for i, delay := range waits {
		if delay > retryMaxDelay {
			t.Fatalf("wait %d = %s, exceeds cap %s", i, delay, retryMaxDelay)
		}
	}
	if waits[len(waits)-1] != retryMaxDelay {
		t.Fatalf("last wait = %s, want cap %s", waits[len(waits)-1], retryMaxDelay)
	}
	if len(log.errs) != 12 || log.infos != 2 {
		t.Fatalf("logs: infos=%d errors=%d, want two infos and 12 errors", log.infos, len(log.errs))
	}
}

func TestJob_RunLogsRefreshFailureWithoutCompletion(t *testing.T) {
	log := &stubLogger{}
	refreshErr := errors.New("db gone")
	job := newTestJob(t,
		syncerFunc(func(context.Context) error { return nil }),
		refresherFunc(func() error { return refreshErr }),
		log,
		func(context.Context, time.Duration) error { return nil },
	)

	job.Run(context.Background())

	if len(log.errs) != 1 || !errors.Is(log.errs[0], refreshErr) {
		t.Fatalf("errors = %v, want wrapped refresh error", log.errs)
	}
	if log.infos != 1 {
		t.Fatalf("info calls = %d, want startup only", log.infos)
	}
}

func TestJob_RunWithPreCanceledContextSkipsAllWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	log := &stubLogger{}
	syncCalls, refreshCalls, waitCalls := 0, 0, 0
	job := newTestJob(t,
		syncerFunc(func(context.Context) error { syncCalls++; return nil }),
		refresherFunc(func() error { refreshCalls++; return nil }),
		log,
		func(context.Context, time.Duration) error { waitCalls++; return nil },
	)

	job.Run(ctx)

	if syncCalls != 0 || refreshCalls != 0 || waitCalls != 0 {
		t.Fatalf("pre-canceled run did work: sync=%d refresh=%d wait=%d", syncCalls, refreshCalls, waitCalls)
	}
	if log.infos != 0 || len(log.errs) != 0 {
		t.Fatalf("pre-canceled run logged: infos=%d errors=%v", log.infos, log.errs)
	}
}

func TestJob_RunCancellationDuringBackoffStopsRetriesAndRefresh(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	log := &stubLogger{}
	syncCalls, refreshCalls := 0, 0
	job := newTestJob(t,
		syncerFunc(func(context.Context) error { syncCalls++; return errors.New("network down") }),
		refresherFunc(func() error { refreshCalls++; return nil }),
		log,
		func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() },
	)

	job.Run(ctx)

	if syncCalls != 1 || refreshCalls != 0 {
		t.Fatalf("calls after cancellation: sync=%d refresh=%d", syncCalls, refreshCalls)
	}
	if len(log.errs) != 1 || log.infos != 1 {
		t.Fatalf("logs: infos=%d errors=%v", log.infos, log.errs)
	}
}

func TestJob_RunCancellationDuringSyncIsQuiet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	log := &stubLogger{}
	refreshCalls := 0
	job := newTestJob(t,
		syncerFunc(func(ctx context.Context) error { cancel(); return ctx.Err() }),
		refresherFunc(func() error { refreshCalls++; return nil }),
		log,
		func(context.Context, time.Duration) error { t.Fatal("wait must not run"); return nil },
	)

	job.Run(ctx)

	if refreshCalls != 0 || len(log.errs) != 0 || log.infos != 1 {
		t.Fatalf("canceled sync: refresh=%d infos=%d errors=%v", refreshCalls, log.infos, log.errs)
	}
}

func TestJob_RunCancellationDuringRefreshSkipsCompletionLog(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	log := &stubLogger{}
	job := newTestJob(t,
		syncerFunc(func(context.Context) error { return nil }),
		refresherFunc(func() error { cancel(); return nil }),
		log,
		func(context.Context, time.Duration) error { t.Fatal("wait must not run"); return nil },
	)

	job.Run(ctx)

	if log.infos != 1 || len(log.errs) != 0 {
		t.Fatalf("logs after refresh cancellation: infos=%d errors=%v", log.infos, log.errs)
	}
}

func TestWaitForRetry_PreCanceledReturnsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := waitForRetry(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForRetry error = %v, want context.Canceled", err)
	}
}

type pointerSyncer struct{}

func (*pointerSyncer) Sync(context.Context) error { return nil }

type pointerRefresher struct{}

func (*pointerRefresher) UpdateFromDb() error { return nil }

func TestNewRejectsMissingDependencies(t *testing.T) {
	validSyncer := syncerFunc(func(context.Context) error { return nil })
	validRefresher := refresherFunc(func() error { return nil })
	validLogger := &stubLogger{}

	var typedNilSyncer *pointerSyncer
	var typedNilRefresher *pointerRefresher
	var typedNilLogger *stubLogger
	tests := []struct {
		name      string
		syncer    Syncer
		refresher FilterRefresher
		logger    Logger
		want      string
	}{
		{name: "nil syncer", refresher: validRefresher, logger: validLogger, want: "syncer"},
		{name: "typed nil syncer", syncer: typedNilSyncer, refresher: validRefresher, logger: validLogger, want: "syncer"},
		{name: "nil refresher", syncer: validSyncer, logger: validLogger, want: "refresher"},
		{name: "typed nil refresher", syncer: validSyncer, refresher: typedNilRefresher, logger: validLogger, want: "refresher"},
		{name: "nil logger", syncer: validSyncer, refresher: validRefresher, want: "logger"},
		{name: "typed nil logger", syncer: validSyncer, refresher: validRefresher, logger: typedNilLogger, want: "logger"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.syncer, tt.refresher, tt.logger)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New error = %v, want dependency name %q", err, tt.want)
			}
		})
	}
}
