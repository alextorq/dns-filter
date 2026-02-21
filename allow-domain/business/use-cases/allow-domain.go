package allow_domain_use_cases

import (
	"fmt"
	"time"

	"github.com/alextorq/dns-filter/allow-domain/db"
	"github.com/alextorq/dns-filter/logger"
)

type AllowDomainEventStore struct {
	buf      []string
	ch       chan string
	capacity int
}

func CreateAllowDomainEventStore(capacity int) *AllowDomainEventStore {
	s := &AllowDomainEventStore{
		buf:      make([]string, 0),
		ch:       make(chan string, 5000),
		capacity: capacity,
	}

	go s.start()

	return s
}

func (e *AllowDomainEventStore) start() {
	listen := func() {
		domains := e.buf
		e.buf = make([]string, 0, e.capacity)
		err := db.CreateBatchDomains(domains)
		if err != nil {
			l := logger.GetLogger()
			l.Error(fmt.Errorf("ошибка отправки события о разрешенном домене: %w", err))
		}
	}
	ticker := time.NewTicker(5 * time.Minute)
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

func (e *AllowDomainEventStore) SendAllowDomainEvent(domain string) {
	select {
	case e.ch <- domain:
	default:
		// Если канал переполнен, лучше потерять лог, чем заблокировать DNS запрос
		logger.GetLogger().Warn("Allow event channel full, dropping event for: " + domain)
	}
}
