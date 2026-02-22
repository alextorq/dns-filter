package blocked_domain_use_cases_block_domain

import (
	"fmt"
	"time"

	blacklists "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/logger"
)

type BlockDomainEventStore struct {
	buf      []string
	ch       chan string
	capacity int
}

func CreateBlockDomainEventStore(capacity int) *BlockDomainEventStore {
	s := &BlockDomainEventStore{
		buf:      make([]string, 0),
		ch:       make(chan string, 5000),
		capacity: capacity,
	}

	go s.start()

	return s
}

func (e *BlockDomainEventStore) start() {
	listen := func() {
		domains := e.buf
		e.buf = make([]string, 0, e.capacity)
		err := blacklists.BatchCreateBlockDomainEvents(domains)
		if err != nil {
			l := logger.GetLogger()
			l.Error(fmt.Errorf("error processing batch block events: %w", err))
		}
	}
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case domain := <-e.ch:
			e.buf = append(e.buf, domain)
			if len(e.buf) >= e.capacity {
				listen()
			}
		case <-ticker.C:
			if len(e.buf) != 0 {
				listen()
			}
		}
	}
}

func (e *BlockDomainEventStore) SendBlockDomainEvent(domain string) {
	select {
	case e.ch <- domain:
	default:
		// Если канал переполнен, лучше потерять лог, чем заблокировать DNS запрос
		logger.GetLogger().Warn("Block event channel full, dropping event for: " + domain)
	}
}
