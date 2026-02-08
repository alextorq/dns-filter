package blocked_domain_use_cases_block_domain

import (
	"fmt"
	"time"

	blacklists "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/logger"
	dnsLib "github.com/miekg/dns"
)

const (
	batchSize     = 100
	flushInterval = 20 * time.Second
	chanSize      = 5000
)

var eventChan = make(chan string, chanSize)

func StartWorker() {
	l := logger.GetLogger()
	buffer := make([]string, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	saveFunc := func() {
		if len(buffer) == 0 {
			return
		}
		if err := blacklists.BatchCreateBlockDomainEvents(buffer); err != nil {
			l.Error(fmt.Errorf("error processing batch block events: %w", err))
		}
		// Очищаем буфер, сохраняя capacity
		buffer = buffer[:0]
	}

	l.Info("Block event worker started")

	for {
		select {
		case domain := <-eventChan:
			buffer = append(buffer, domain)
			if len(buffer) >= batchSize {
				saveFunc()
			}
		case <-ticker.C:
			saveFunc()
		}
	}
}

func BlockDomain(_ dnsLib.ResponseWriter, r *dnsLib.Msg) {
	if len(r.Question) == 0 {
		return
	}
	first := r.Question[0]
	domain := first.Name

	select {
	case eventChan <- domain:
	default:
		// Если канал переполнен, лучше потерять лог, чем заблокировать DNS запрос
		logger.GetLogger().Warn("Block event channel full, dropping event for: " + domain)
	}
}
