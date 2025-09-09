package main

import (
	"dns-filter/cache"
	"dns-filter/filter"
	"dns-filter/metric"
	usecases "dns-filter/use-cases"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/miekg/dns"
)

var blackList *bloom.BloomFilter = nil
var cacheInstance *cache.LRUCache = nil

func GetFromCacheOrCreateRequest(question dns.Question, id uint16) (r *dns.Msg, err error) {
	qtype := dns.TypeToString[question.Qtype]
	name := question.Name
	cacheKey := name + ":" + qtype

	// Сначала проверяем кэш
	fromCache, found := cacheInstance.Get(cacheKey)
	if found {
		fmt.Println("Из кэша:", name, "Тип:", qtype)
		// Возвращаем кэшированный ответ
		return fromCache, nil
	}

	// Если не в блоклисте → ходим на апстрим
	resp, err := dns.Exchange(&dns.Msg{
		MsgHdr:   dns.MsgHdr{Id: id, RecursionDesired: true},
		Question: []dns.Question{question},
	}, "8.8.8.8:53")

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

		fmt.Println("Запрос:", qname, "Тип:", qtype)

		if blackList.Test([]byte(qname)) {
			// Блокируем → NXDOMAIN
			m.Rcode = dns.RcodeNameError
			blocked = true
			fmt.Println("Заблокирован:", qname)
		} else {
			resp, err := GetFromCacheOrCreateRequest(q, r.Id)

			if err != nil {
				log.Println("Ошибка апстрима:", err)
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
		metric.HandleDNSRequest(clientIP, qtype, dns.RcodeToString[m.Rcode], respSize, duration, blocked)
	}

	if err := w.WriteMsg(m); err != nil {
		log.Println("Ошибка отправки:", err)
	}
}

func main() {
	err := usecases.GetFromDb()
	blackList = filter.GetFilter()
	cacheInstance = cache.GetCache()
	usecases.StartMetric()

	if err != nil {
		log.Fatal("Ошибка синхронизации блоклиста:", err)
	}

	dns.HandleFunc(".", handleDNS)

	server := &dns.Server{Addr: ":53", Net: "udp"}
	fmt.Println("DNS фильтр запущен на :53")
	log.Fatal(server.ListenAndServe())
}
