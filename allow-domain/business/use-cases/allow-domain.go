package allow_domain_use_cases

import (
	"fmt"
	"time"
)

// Repo is the output port: persisting batched allow events.
type Repo interface {
	CreateBatch(domains []string) error
}

type Logger interface {
	Warn(args ...any)
	Error(err error)
}

type AllowDomainEventStore struct {
	repo     Repo
	log      Logger
	buf      []string
	ch       chan string
	capacity int
}

// defaultChannelSize is the inbox buffer for SendAllowDomainEvent. Sized to
// absorb DNS bursts without dropping events during a normal flush window.
const defaultChannelSize = 5000

// CreateAllowDomainEventStore starts a background worker that accumulates
// allow events and flushes them to the repo either when capacity is reached
// or on a 5-minute ticker. The hot DNS path must never block on a DB write —
// see SendAllowDomainEvent.
func CreateAllowDomainEventStore(repo Repo, log Logger, capacity int) *AllowDomainEventStore {
	return newWithChannelSize(repo, log, capacity, defaultChannelSize)
}

// newWithChannelSize is the test seam: exposes the inbox buffer so a unit
// test can force the "channel full → drop" branch deterministically without
// flooding 5000 messages.
func newWithChannelSize(repo Repo, log Logger, capacity, chanSize int) *AllowDomainEventStore {
	s := &AllowDomainEventStore{
		repo:     repo,
		log:      log,
		buf:      make([]string, 0),
		ch:       make(chan string, chanSize),
		capacity: capacity,
	}
	go s.start()
	return s
}

func (e *AllowDomainEventStore) start() {
	flush := func() {
		domains := e.buf
		e.buf = make([]string, 0, e.capacity)
		if err := e.repo.CreateBatch(domains); err != nil {
			e.log.Error(fmt.Errorf("error processing batch allow events: %w", err))
		}
	}
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case domain := <-e.ch:
			e.buf = append(e.buf, domain)
			if len(e.buf) >= e.capacity {
				flush()
			}
		case <-ticker.C:
			if len(e.buf) != 0 {
				flush()
			}
		}
	}
}

func (e *AllowDomainEventStore) SendAllowDomainEvent(domain string) {
	select {
	case e.ch <- domain:
	default:
		// Channel full — drop event rather than block the DNS hot path.
		e.log.Warn("Allow event channel full, dropping event for: " + domain)
	}
}
