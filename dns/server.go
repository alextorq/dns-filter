package dns

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/alextorq/dns-filter/config"
	usecases "github.com/alextorq/dns-filter/use-cases"
	"github.com/miekg/dns"
)

var conf = config.GetConfig()

type DnsServer struct {
	Address string
	Port    int
	server  *dns.Server
	Logger  Logger
	Cache   Cache
	Filter  func(string2 string) bool
	Metric  Metric
}

type Logger interface {
	Info(args ...interface{})
	Error(err error)
	Debug(args ...interface{})
	Warn(args ...interface{})
}

type Cache interface {
	Get(key string) (*dns.Msg, bool)
	Add(key string, val *dns.Msg)
}

type Metric interface {
	HandleDNSRequest(clientIP, qtype, rcode string, respSize int, duration time.Duration)
}

type Filter interface {
	CheckBlock(domain string) bool
}

func (s *DnsServer) GetFromCacheOrCreateRequest(question dns.Question, id uint16) (r *dns.Msg, err error) {
	qtype := dns.TypeToString[question.Qtype]
	name := question.Name
	cacheKey := name + ":" + qtype

	// Сначала проверяем кэш
	fromCache, found := s.Cache.Get(cacheKey)
	if found {
		s.Logger.Info("Из кэша:", name, "Тип:", qtype)
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
	s.Cache.Add(cacheKey, resp.Copy())
	return resp, nil
}

func (s *DnsServer) handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)

	start := time.Now()
	clientIP, _, _ := net.SplitHostPort(w.RemoteAddr().String())

	for _, q := range r.Question {
		qtype := dns.TypeToString[q.Qtype]
		qname := q.Name

		if s.Filter(qname) {
			// Блокируем → NXDOMAIN
			m.Rcode = dns.RcodeNameError
			usecases.BlockDomain(qname)
		} else {
			s.Logger.Debug("Запрос:", qname, "Тип:", qtype)
			resp, err := s.GetFromCacheOrCreateRequest(q, r.Id)
			if err != nil {
				s.Logger.Error(fmt.Errorf("ошибка апстрима для %s: %w", qname, err))
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
		s.Metric.HandleDNSRequest(clientIP, qtype, dns.RcodeToString[m.Rcode], respSize, duration)
	}

	if err := w.WriteMsg(m); err != nil {
		s.Logger.Error(fmt.Errorf("ошибка отправки ответа клиенту: %w", err))
	}
}

func (s *DnsServer) Serve() {
	dns.HandleFunc(".", func(writer dns.ResponseWriter, msg *dns.Msg) {
		s.handleDNS(writer, msg)
	})
	s.server = &dns.Server{Addr: ":53", Net: "udp"}
	s.Logger.Info("DNS фильтр запущен на :53")
	log.Fatal(s.server.ListenAndServe())
}

func CreateServer(logger Logger, cache Cache, filter func(string2 string) bool, metric Metric) *DnsServer {
	return &DnsServer{
		Logger: logger,
		Cache:  cache,
		Filter: filter,
		Metric: metric,
	}
}
