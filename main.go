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
	//rcode := dns.RcodeSuccess

	for _, q := range r.Question {
		fmt.Println("Запрос:", q.Name, "Тип:", dns.TypeToString[q.Qtype])

		qtype := dns.TypeToString[q.Qtype]
		qname := q.Name

		if blackList.Test([]byte(qname)) {
			// Блокируем → NXDOMAIN
			m.Rcode = dns.RcodeNameError
			blocked = true
			//rcode = dns.RcodeNameError
			fmt.Println("Заблокирован:", q.Name)
		} else {
			// Если не в блоклисте → ходим на апстрим
			resp, err := dns.Exchange(&dns.Msg{
				MsgHdr:   dns.MsgHdr{Id: r.Id, RecursionDesired: true},
				Question: []dns.Question{q},
			}, "8.8.8.8:53")

			if err != nil {
				log.Println("Ошибка апстрима:", err)
				m.Rcode = dns.RcodeServerFailure
				//rcode = dns.RcodeServerFailure
			} else {
				// Добавляем все ответы из апстрима в общий ответ
				m.Answer = append(m.Answer, resp.Answer...)
				m.Ns = append(m.Ns, resp.Ns...)
				m.Extra = append(m.Extra, resp.Extra...)

				// метрики для успешного апстрима
				duration := time.Since(start)
				respSize := resp.Len()
				metric.HandleDNSRequest(clientIP, qtype, dns.RcodeToString[resp.Rcode], respSize, duration, false)
			}
		}
	}

	// В конце отправляем общий ответ клиенту
	duration := time.Since(start)
	respSize := m.Len()
	metric.HandleDNSRequest(clientIP, "multi", dns.RcodeToString[m.Rcode], respSize, duration, blocked)

	if err := w.WriteMsg(m); err != nil {
		log.Println("Ошибка отправки:", err)
	}
}

func main() {
	//err := use_cases.LoadFromFile()
	//if err != nil {
	//	fmt.Println(err)
	//}
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
