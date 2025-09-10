package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/alextorq/dns-filter/cache"
	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/db/migrate"
	"github.com/alextorq/dns-filter/logger"
	"github.com/alextorq/dns-filter/metric"
	usecases "github.com/alextorq/dns-filter/use-cases"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/miekg/dns"
)

var blackList *bloom.BloomFilter = nil
var cacheInstance = cache.GetCache()
var l = logger.GetLogger()
var conf = config.GetConfig()
var metricInstance = metric.CreateMetric(conf.MetricEnable, conf.MetricPort).Serve()

func GetFromCacheOrCreateRequest(question dns.Question, id uint16) (r *dns.Msg, err error) {
	qtype := dns.TypeToString[question.Qtype]
	name := question.Name
	cacheKey := name + ":" + qtype

	// Сначала проверяем кэш
	fromCache, found := cacheInstance.Get(cacheKey)
	if found {
		l.Info("Из кэша:", name, "Тип:", qtype)
		// Возвращаем кэшированный ответ
		return fromCache, nil
	}

	// Если не в блоклисте → ходим на апстрим
	resp, err := dns.Exchange(&dns.Msg{
		MsgHdr:   dns.MsgHdr{Id: id, RecursionDesired: true},
		Question: []dns.Question{question},
	}, conf.Upstream)

	if err != nil {
		return nil, err
	}

	// Кладем в кэш
	cacheInstance.Add(cacheKey, resp.Copy())
	return resp, nil
}

func handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)

	start := time.Now()
	clientIP, _, _ := net.SplitHostPort(w.RemoteAddr().String())

	blocked := false

	for _, q := range r.Question {
		qtype := dns.TypeToString[q.Qtype]
		qname := q.Name

		if blackList.Test([]byte(qname)) {
			// Блокируем → NXDOMAIN
			m.Rcode = dns.RcodeNameError
			blocked = true
			err := usecases.BlockDomain(qname)
			if err != nil {
				l.Error(fmt.Errorf("ошибка блокировки домена %s: %w", qname, err))
			}
		} else {
			resp, err := GetFromCacheOrCreateRequest(q, r.Id)
			l.Debug("Запрос:", qname, "Тип:", qtype)
			if err != nil {
				l.Error(fmt.Errorf("ошибка апстрима для %s: %w", qname, err))
				m.Rcode = dns.RcodeServerFailure
			} else {
				// Добавляем все ответы из апстрима в общий ответ
				m.Answer = append(m.Answer, resp.Answer...)
				m.Ns = append(m.Ns, resp.Ns...)
				m.Extra = append(m.Extra, resp.Extra...)
			}
		}

		// В конце отправляем общий ответ клиенту
		duration := time.Since(start)
		respSize := m.Len()
		metricInstance.HandleDNSRequest(clientIP, qtype, dns.RcodeToString[m.Rcode], respSize, duration, blocked)
	}

	if err := w.WriteMsg(m); err != nil {
		l.Error(fmt.Errorf("ошибка отправки ответа клиенту: %w", err))
	}
}

func main() {
	migrate.Migrate()
	usecases.StartCleanUpBlockDomain()
	filter, err := usecases.GetFromDb()
	if err != nil {
		l.Error(err)
		panic(err)
	}
	blackList = filter

	dns.HandleFunc(".", handleDNS)
	server := &dns.Server{Addr: ":53", Net: "udp"}
	l.Info("DNS фильтр запущен на :53")
	log.Fatal(server.ListenAndServe())
}
