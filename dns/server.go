package dns

import (
	"context"
	"errors"
	"fmt"
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
	udp      *dns.Server
	tcp      *dns.Server
	Logger   Logger
	Cache    Cache
	Filter   func(string2 string) bool
	Upstream UpstreamResolver
	Metric   Metric
	Handlers DnsRequestHandlers
	// NotifyStartedFunc is invoked once for each underlying listener (UDP
	// and TCP) right after it becomes ready to accept queries. Optional;
	// primarily for tests that need to wait for both listeners.
	NotifyStartedFunc func()
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

// Serve binds UDP and TCP listeners on s.Address (default ":53") and runs
// both in parallel. RFC 7766 requires DNS servers to support TCP for clients
// that retry after a truncated UDP response.
func (s *DnsServer) Serve() error {
	addr := s.Address
	if addr == "" {
		addr = ":53"
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("resolve udp addr: %w", err)
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("listen udp: %w", err)
	}
	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		udpConn.Close()
		return fmt.Errorf("listen tcp: %w", err)
	}

	for _, ip := range utils.GetIp() {
		s.Logger.Info("DNS фильтр запущен на:", ip+addr)
	}

	return s.ServeWithListeners(udpConn, tcpListener)
}

// ServeWithListeners runs UDP and TCP DNS servers on pre-bound endpoints.
// It blocks until both servers exit. If one server returns an error, the
// other is shut down so the process never sits in a half-up state where
// e.g. UDP is dead but TCP keeps answering. Exposed so tests can bind to
// an ephemeral port without racing the OS.
func (s *DnsServer) ServeWithListeners(udpConn net.PacketConn, tcpListener net.Listener) error {
	handler := dns.HandlerFunc(func(w dns.ResponseWriter, m *dns.Msg) {
		s.handleDNS(w, m)
	})
	s.udp = &dns.Server{PacketConn: udpConn, Handler: handler, NotifyStartedFunc: s.NotifyStartedFunc}
	s.tcp = &dns.Server{Listener: tcpListener, Handler: handler, NotifyStartedFunc: s.NotifyStartedFunc}

	errCh := make(chan error, 2)
	go func() { errCh <- s.udp.ActivateAndServe() }()
	go func() { errCh <- s.tcp.ActivateAndServe() }()

	first := <-errCh
	_ = s.Shutdown()
	<-errCh
	return first
}

// Shutdown gracefully stops both UDP and TCP listeners.
func (s *DnsServer) Shutdown() error {
	var err error
	if s.udp != nil {
		err = errors.Join(err, s.udp.Shutdown())
	}
	if s.tcp != nil {
		err = errors.Join(err, s.tcp.Shutdown())
	}
	return err
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
