package logger

import (
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
		level:   DEBUG,
	}
	// No goroutine drains logChan. Saturate it first.
	for range buf {
		l.logChan <- log.LogStruct{Level: "INFO", Message: "filler", Time: time.Now()}
	}

	done := make(chan struct{})
	go func() {
		// Several extra sends — every one must return immediately.
		for range 10 {
			l.Info("must not block")
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("logger.send blocked when channel was full")
	}
}
