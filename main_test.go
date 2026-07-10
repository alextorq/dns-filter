package main

import (
	"errors"
	"net/http"
	"sync"
	"testing"
)

type stubServerLogger struct {
	mu   sync.Mutex
	errs []error
}

func (l *stubServerLogger) Error(err error) {
	l.mu.Lock()
	l.errs = append(l.errs, err)
	l.mu.Unlock()
}

func TestReportServerError_IgnoresExpectedShutdown(t *testing.T) {
	log := &stubServerLogger{}
	reportServerError("metrics", http.ErrServerClosed, log)
	if len(log.errs) != 0 {
		t.Fatalf("expected shutdown must be quiet, got %v", log.errs)
	}
}

func TestReportServerError_LogsUnexpectedFailure(t *testing.T) {
	log := &stubServerLogger{}
	boom := errors.New("bind failed")
	reportServerError("metrics", boom, log)
	if len(log.errs) != 1 || !errors.Is(log.errs[0], boom) {
		t.Fatalf("logged errors = %v, want wrapped bind failure", log.errs)
	}
}
