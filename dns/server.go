package dns

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	clients "github.com/alextorq/dns-filter/clients/client"
	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/utils"
	"github.com/miekg/dns"
)

type DnsServer struct {
	Address  string
	Port     int
	server   *dns.Server
	Logger   Logger
	Cache    Cache
	Filter   func(string2 string) bool
	Upstream UpstreamResolver
	Metric   Metric
	Handlers DnsRequestHandlers
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

type DnsRequestHandlers interface {
	Allowed(w dns.ResponseWriter, r *dns.Msg)
	Blocked(w dns.ResponseWriter, r *dns.Msg)
}

type Filter interface {
	CheckBlock(domain string) bool
}

func (s *DnsServer) GetFromCacheOrCreateRequest(ctx context.Context, question dns.Question, id uint16) (r *dns.Msg, err error) {
	qtype := dns.TypeToString[question.Qtype]
	name := question.Name
	cacheKey := name + ":" + qtype

	// Сначала проверяем кэш
	fromCache, found := s.Cache.Get(cacheKey)
	if found {
		s.Logger.Debug("Из кэша:", name, "Тип:", qtype)
		cached := fromCache.Copy()
		cached.Id = id
		return cached, nil
	}

	resp, err := s.Upstream.Exchange(ctx, &dns.Msg{
		MsgHdr:   dns.MsgHdr{Id: id, RecursionDesired: true},
		Question: []dns.Question{question},
	})

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
	cl := clients.GetClients()

	start := time.Now()
	clientIP, _, _ := net.SplitHostPort(w.RemoteAddr().String())

	for _, q := range r.Question {
		qtype := dns.TypeToString[q.Qtype]
		qname := q.Name

		useFilter := s.Filter(qname)
		if cl.ClientExist(clientIP) {
			useFilter = false
			s.Logger.Debug("Клиент: ", clientIP, "из списка разрешённых")
		}

		if useFilter {
			// Блокируем → NXDOMAIN
			m.Rcode = dns.RcodeNameError
			s.Handlers.Blocked(w, r)
		} else {
			s.Logger.Debug("Запрос:", qname, "Тип:", qtype)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			resp, err := s.GetFromCacheOrCreateRequest(ctx, q, r.Id)
			cancel()
			if err != nil {
				s.Logger.Error(fmt.Errorf("ошибка апстрима для %s: %w", qname, err))
				m.Rcode = dns.RcodeServerFailure
			} else {
				m.Rcode = resp.Rcode
				// Добавляем все ответы из апстрима в общий ответ
				m.Answer = append(m.Answer, resp.Answer...)
				m.Ns = append(m.Ns, resp.Ns...)
				m.Extra = append(m.Extra, resp.Extra...)
			}
			s.Handlers.Allowed(w, r)
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
	ips := utils.GetIp()
	for _, ip := range ips {
		s.Logger.Info("DNS фильтр запущен на:", ip+":53")
	}
	log.Fatal(s.server.ListenAndServe())
}

func CreateServer(logger Logger, cache Cache, filter func(string2 string) bool, metric Metric, handlers DnsRequestHandlers) *DnsServer {
	conf := config.GetConfig()
	return CreateServerWithResolver(logger, cache, filter, metric, handlers, NewDoHResolver(conf.DoHUpstream, conf.DoHBootstrapIPs...))
}

func CreateServerWithResolver(logger Logger, cache Cache, filter func(string2 string) bool, metric Metric, handlers DnsRequestHandlers, upstream UpstreamResolver) *DnsServer {
	return &DnsServer{
		Logger:   logger,
		Cache:    cache,
		Filter:   filter,
		Upstream: upstream,
		Metric:   metric,
		Handlers: handlers,
	}
}
