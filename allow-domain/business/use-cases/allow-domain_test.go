package allow_domain_use_cases

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeRepo struct {
	mu      sync.Mutex
	batches [][]string
	err     error
}

func (f *fakeRepo) CreateBatch(domains []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	batch := make([]string, len(domains))
	copy(batch, domains)
	f.batches = append(f.batches, batch)
	return f.err
}

func (f *fakeRepo) snapshot() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, len(f.batches))
	copy(out, f.batches)
	return out
}

type recordingLog struct {
	warns atomic.Int32
	errs  atomic.Int32
}

func (l *recordingLog) Warn(args ...any) { l.warns.Add(1) }
func (l *recordingLog) Error(err error)  { l.errs.Add(1) }

func waitFor(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}

func TestEventStore_FlushesOnCapacity(t *testing.T) {
	repo := &fakeRepo{}
	store := CreateAllowDomainEventStore(repo, &recordingLog{}, 2)

	store.SendAllowDomainEvent("a")
	store.SendAllowDomainEvent("b") // hits capacity

	waitFor(t, time.Second, "first flush", func() bool {
		return len(repo.snapshot()) >= 1
	})

	batches := repo.snapshot()
	if len(batches[0]) != 2 {
		t.Errorf("expected first batch size 2, got %v", batches[0])
	}
}

func TestEventStore_LogsRepoError(t *testing.T) {
	repo := &fakeRepo{err: errors.New("db down")}
	log := &recordingLog{}
	store := CreateAllowDomainEventStore(repo, log, 1)

	store.SendAllowDomainEvent("oops")

	waitFor(t, time.Second, "error logged", func() bool {
		return log.errs.Load() >= 1
	})
}

// blockingRepo lets the test stall the worker inside the flush so the inbox
// channel saturates. enter signals the worker entered the flush, exit holds
// it there until the test releases it.
type blockingRepo struct {
	enter chan struct{}
	exit  chan struct{}
}

func (b *blockingRepo) CreateBatch(domains []string) error {
	b.enter <- struct{}{}
	<-b.exit
	return nil
}

// SendAllowDomainEvent must DROP (not block) when the inbox is saturated, and
// log a warning instead. The hot DNS path can never afford to be backpressured
// by a slow/stuck DB. Without this guarantee a stuck flush would back up onto
// every DNS query that was allowed through.
func TestEventStore_DropsWhenChannelFull(t *testing.T) {
	repo := &blockingRepo{enter: make(chan struct{}, 1), exit: make(chan struct{})}
	log := &recordingLog{}
	// capacity=1 forces a flush after the first event; chanSize=1 means a
	// single extra send saturates the inbox while the worker is blocked.
	store := newWithChannelSize(repo, log, 1, 1)

	store.SendAllowDomainEvent("first")
	// Happens-before via the enter channel: the worker reads "first" from
	// e.ch BEFORE it can send into b.enter (both happen sequentially in the
	// worker goroutine). So by the time <-repo.enter returns, e.ch is
	// guaranteed empty and the worker is parked on <-b.exit inside the flush.
	<-repo.enter
	// Channel drained — fill the 1-slot buffer.
	store.SendAllowDomainEvent("second")
	// Third send must hit the default branch → warn + drop.
	store.SendAllowDomainEvent("dropped")

	waitFor(t, time.Second, "warn for drop", func() bool {
		return log.warns.Load() >= 1
	})

	// Release the worker so it doesn't leak past the test.
	close(repo.exit)
}
