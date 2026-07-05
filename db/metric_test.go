package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type monitorTestLogger struct {
	errs []error
}

func (l *monitorTestLogger) Debug(...any)    {}
func (l *monitorTestLogger) Error(err error) { l.errs = append(l.errs, err) }

type fakeFileInfo struct{ size int64 }

func (f fakeFileInfo) Name() string       { return "filter.sqlite" }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() os.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }

func TestDBSizeMonitor_RunPreCanceledSkipsStat(t *testing.T) {
	calls := 0
	m, err := newDBSizeMonitor(
		prometheus.NewRegistry(),
		"filter.sqlite",
		&monitorTestLogger{},
		time.Hour,
		func(string) (os.FileInfo, error) { calls++; return fakeFileInfo{}, nil },
	)
	if err != nil {
		t.Fatalf("new monitor: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m.Run(ctx)

	if calls != 0 {
		t.Fatalf("stat calls = %d, want 0 for pre-canceled context", calls)
	}
}

func TestDBSizeMonitor_RunObservesImmediatelyAndStopsOnCancel(t *testing.T) {
	started := make(chan struct{})
	m, err := newDBSizeMonitor(
		prometheus.NewRegistry(),
		"filter.sqlite",
		&monitorTestLogger{},
		time.Hour,
		func(string) (os.FileInfo, error) {
			close(started)
			return fakeFileInfo{size: 1234}, nil
		},
	)
	if err != nil {
		t.Fatalf("new monitor: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()

	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitor did not stop after cancellation")
	}
	if got := testutil.ToFloat64(m.gauge); got != 1234 {
		t.Fatalf("gauge = %v, want 1234", got)
	}
}

func TestDBSizeMonitor_ObserveLogsStatError(t *testing.T) {
	boom := errors.New("stat failed")
	log := &monitorTestLogger{}
	m, err := newDBSizeMonitor(
		prometheus.NewRegistry(),
		"missing.sqlite",
		log,
		time.Hour,
		func(string) (os.FileInfo, error) { return nil, boom },
	)
	if err != nil {
		t.Fatalf("new monitor: %v", err)
	}

	m.observe()

	if len(log.errs) != 1 || !errors.Is(log.errs[0], boom) {
		t.Fatalf("logged errors = %v, want wrapped stat error", log.errs)
	}
}

func TestNewDBSizeMonitor_RejectsNonPositiveInterval(t *testing.T) {
	_, err := NewDBSizeMonitor(
		prometheus.NewRegistry(),
		"filter.sqlite",
		&monitorTestLogger{},
		0,
	)
	if err == nil {
		t.Fatal("expected non-positive interval to be rejected")
	}
}

func TestNewDBSizeMonitor_DuplicateRegistrationReturnsError(t *testing.T) {
	registry := prometheus.NewRegistry()
	if _, err := NewDBSizeMonitor(registry, "first.sqlite", &monitorTestLogger{}, time.Hour); err != nil {
		t.Fatalf("first monitor: %v", err)
	}
	if _, err := NewDBSizeMonitor(registry, "second.sqlite", &monitorTestLogger{}, time.Hour); err == nil {
		t.Fatal("expected duplicate metric registration error")
	}
}
