package main

import (
	"dns-filter/filter"
	"dns-filter/metric"
	use_cases "dns-filter/use-cases"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/miekg/dns"
)

var blackList *bloom.BloomFilter = nil

func handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)

	start := time.Now()
	clientIP, _, _ := net.SplitHostPort(w.RemoteAddr().String())

	blocked := false
	rcode := dns.RcodeToString[dns.RcodeSuccess]

	// Разбираем все вопросы
	for _, q := range r.Question {
		fmt.Println("Запрос:", q.Name)
		qtype := dns.TypeToString[q.Qtype]
		qname := q.Name

		if blackList.Test([]byte(qname)) {
			// Если в блоклисте → возвращаем NXDOMAIN
			m.Rcode = dns.RcodeNameError
			blocked = true
			rcode = dns.RcodeToString[m.Rcode]
		} else {
			// Иначе → пересылаем на апстрим (Google DNS)
			resp, err := dns.Exchange(r, "8.8.8.8:53")
			if err != nil {
				log.Println("Ошибка апстрима:", err)
				m.Rcode = dns.RcodeServerFailure
				rcode = dns.RcodeToString[m.Rcode]
			} else {
				// метрики для успешного апстрима
				duration := time.Since(start)
				respSize := resp.Len()
				metric.HandleDNSRequest(clientIP, qtype, dns.RcodeToString[resp.Rcode], respSize, duration, false)

				w.WriteMsg(resp)
				return
			}
		}

		duration := time.Since(start)
		respSize := m.Len()
		// 👉 вот здесь вызов нашей метрики
		metric.HandleDNSRequest(clientIP, qtype, rcode, respSize, duration, blocked)
	}

	// отправляем ответ
	err := w.WriteMsg(m)
	if err != nil {
		log.Println("Ошибка отправки:", err)
	}
}

func main() {
	err := use_cases.GetFromDb()
	blackList = filter.GetFilter()
	use_cases.StartMetric()

	if err != nil {
		log.Fatal("Ошибка синхронизации блоклиста:", err)
	}

	dns.HandleFunc(".", handleDNS)

	server := &dns.Server{Addr: ":53", Net: "udp"}
	fmt.Println("DNS фильтр запущен на :53")
	log.Fatal(server.ListenAndServe())
}
