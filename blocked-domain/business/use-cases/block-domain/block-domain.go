package blocked_domain_use_cases_block_domain

import (
	"fmt"
	"time"
)

// Repo is the output port: writing batched block events.
type Repo interface {
	BatchCreateBlockDomainEvents(domains []string) error
}

type Logger interface {
	Warn(args ...any)
	Error(err error)
}

type BlockDomainEventStore struct {
	repo     Repo
	log      Logger
	buf      []string
	ch       chan string
	capacity int
}

// defaultChannelSize is the inbox buffer for SendBlockDomainEvent. Sized to
// absorb DNS bursts without dropping events during a normal flush window.
const defaultChannelSize = 5000

// NewBlockDomainEventStore starts a background worker that accumulates block
// events and flushes them to the repo either when capacity is reached or on a
// 20s ticker. The hot DNS path must never block on a DB write — see
// SendBlockDomainEvent.
func NewBlockDomainEventStore(repo Repo, log Logger, capacity int) *BlockDomainEventStore {
	return newWithChannelSize(repo, log, capacity, defaultChannelSize)
}

// newWithChannelSize is the test seam: exposes the inbox buffer so a unit test
// can force the "channel full → drop" branch deterministically without flooding
// 5000 messages.
func newWithChannelSize(repo Repo, log Logger, capacity, chanSize int) *BlockDomainEventStore {
	s := &BlockDomainEventStore{
		repo:     repo,
		log:      log,
		buf:      make([]string, 0),
		ch:       make(chan string, chanSize),
		capacity: capacity,
	}
	go s.start()
	return s
}

func (e *BlockDomainEventStore) start() {
	flush := func() {
		domains := e.buf
		e.buf = make([]string, 0, e.capacity)
		if err := e.repo.BatchCreateBlockDomainEvents(domains); err != nil {
			e.log.Error(fmt.Errorf("error processing batch block events: %w", err))
		}
	}
	ticker := time.NewTicker(20 * time.Second)
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

func (e *BlockDomainEventStore) SendBlockDomainEvent(domain string) {
	select {
	case e.ch <- domain:
	default:
		// Channel full — drop event rather than block the DNS hot path.
		e.log.Warn("Block event channel full, dropping event for: " + domain)
	}
}
