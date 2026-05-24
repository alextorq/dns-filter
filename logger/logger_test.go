package logger

import (
	"sync"
	"testing"
	"time"

	"github.com/alextorq/dns-filter/logger/log"
)

// Locks in the #35 fix: when the channel is full and there is no consumer,
// send must drop the message instead of blocking the caller. Before the fix,
// this test would hang and fail by the t.Fatalf timeout path.
func TestChanLogger_SendDoesNotBlockWhenBufferFull(t *testing.T) {
	const buf = 4
	l := &ChanLogger{
		logChan: make(chan log.LogStruct, buf),
		quit:    make(chan struct{}),
	}
	l.level.Store(int32(DEBUG)) // zero value is DEBUG anyway; explicit for clarity
	// No goroutine drains logChan. Saturate it first.
	for range buf {
		l.logChan <- log.LogStruct{Level: "INFO", Message: "filler", Time: time.Now()}
	}

	done := make(chan struct{})
	const extras = 10
	go func() {
		// Several extra sends — every one must return immediately.
		for range extras {
			l.Info("must not block")
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("logger.send blocked when channel was full")
	}

	if got := l.DroppedCount(); got != extras {
		t.Fatalf("DroppedCount() = %d, want %d", got, extras)
	}
}

// The logger goroutine reads the level on every record while the settings
// module writes it on a runtime change. This must be race-free — run under
// `go test -race`. Before level became atomic this test failed the race
// detector.
func TestChanLogger_ConcurrentUpdateAndLogIsRaceFree(t *testing.T) {
	l := NewChanLogger(100, "INFO")
	defer l.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 2000 {
			l.Info("hot path message")
		}
	}()
	go func() {
		defer wg.Done()
		levels := []string{"DEBUG", "INFO", "WARN", "ERROR"}
		for i := range 2000 {
			l.UpdateLogLevel(levels[i%len(levels)])
		}
	}()
	wg.Wait()

	// Final write must be observable through the public getter.
	l.UpdateLogLevel("WARN")
	if got := l.GetLogLevel(); got != "WARN" {
		t.Errorf("GetLogLevel() = %q, want WARN", got)
	}
}
