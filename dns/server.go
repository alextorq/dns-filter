package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/alextorq/dns-filter/clients/identifier"
	"github.com/alextorq/dns-filter/clients/store"
	"github.com/alextorq/dns-filter/config"
	dns_cache "github.com/alextorq/dns-filter/dns-cache"
	"github.com/alextorq/dns-filter/metric"
	"github.com/alextorq/dns-filter/utils"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

// serveStaleOnError counts responses where upstream failed but a stale-window
// entry rescued us (RFC 8767). A non-zero value means the resolver kept
// answering during a Cloudflare/DoH blip — without SWR these would be SERVFAIL.
var serveStaleOnError = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "dns_serve_stale_on_error_total",
	Help: "DNS responses served from the stale-window because upstream returned an error (RFC 8767)",
})

func init() {
	metric.Registry.MustRegister(serveStaleOnError)
}

// ClientStore is the subset of the in-memory exclusion snapshot the hot path
// needs. Defined as an interface so tests can inject a stub without standing
// up the singleton.
type ClientStore interface {
	IsExcluded(identifier.Lookup) bool
}

type DnsServer struct {
	Address    string
	Port       int
	udp        *dns.Server
	tcp        *dns.Server
	Logger     Logger
	Cache      Cache
	Filter     func(string2 string) bool
	Upstream   UpstreamResolver
	Metric     Metric
	Handlers   DnsRequestHandlers
	Identifier identifier.Identifier
	Clients    ClientStore
	// upstream collapses concurrent identical queries into a single in-flight
	// upstream call. Zero value is ready to use.
	upstream upstreamCoordinator
	// SWREnabled gates proactive stale-while-revalidate: on Stale-state cache
	// lookups, serve immediately and refresh in the background. When false a
	// stale entry falls through to a synchronous upstream call (the cache may
	// still hold stale data — it's used as fallback in serve-stale-on-error).
	SWREnabled bool
	// Refresh is the async refresh worker invoked on Stale hits. Optional —
	// nil disables proactive refresh even if SWREnabled is true.
	Refresh *refreshWorker
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
	// Lookup returns the cached entry along with its state. Fresh and Stale
	// both come with a Msg; Miss and Expired carry only the State so the
	// caller knows whether the slot exists (Expired) or not (Miss).
	//
	// Lookup.Msg MUST be an owned/copied *dns.Msg: the hot path mutates
	// Msg.Id per caller, and concurrent Lookups for the same key would race
	// on a shared pointer. Both production (CacheWithMetrics) and the test
	// memoryCache satisfy this — any future implementation must too.
	Lookup(key string) dns_cache.Lookup
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

	lookup := s.Cache.Lookup(cacheKey)
	switch lookup.State {
	case dns_cache.StateFresh:
		s.Logger.Debug("Из кэша:", name, "Тип:", qtype)
		lookup.Msg.Id = id
		return lookup.Msg, nil
	case dns_cache.StateStale:
		// Proactive SWR: hand the stale answer to the client immediately and
		// fire a non-blocking refresh. If SWR is disabled we deliberately fall
		// through to a synchronous upstream call — but the stale entry is
		// still in the cache and will be used by serve-stale-on-error below
		// if the upstream call fails.
		if s.SWREnabled && s.Refresh != nil {
			s.Refresh.Refresh(cacheKey, question)
			s.Logger.Debug("Stale из кэша + рефреш:", name, "Тип:", qtype)
			lookup.Msg.Id = id
			return lookup.Msg, nil
		}
	}

	// Miss / Expired / Stale-with-SWR-off → synchronous upstream via singleflight.
	resp, err := s.upstream.Do(cacheKey, func() (*dns.Msg, error) {
		// Double-check the cache: a previous in-flight call may have just
		// populated it, in which case we can skip the upstream entirely.
		if lk := s.Cache.Lookup(cacheKey); lk.State == dns_cache.StateFresh {
			return lk.Msg, nil
		}
		out, err := s.Upstream.Exchange(ctx, &dns.Msg{
			MsgHdr:   dns.MsgHdr{Id: id, RecursionDesired: true},
			Question: []dns.Question{question},
		})
		if err != nil {
			return nil, err
		}
		// The cache deep-copies internally and decides whether the
		// response is cacheable (TTL>0, non-SERVFAIL, …).
		s.Cache.Add(cacheKey, out)
		return out, nil
	})
	if err != nil {
		// RFC 8767 serve-stale-on-error: keep answering during upstream
		// trouble, regardless of SWREnabled. We also accept Fresh here —
		// a concurrent refresh may have populated the cache between our
		// miss and our error, and SERVFAIL'ing with a fresh answer next
		// to us would be perverse.
		lk := s.Cache.Lookup(cacheKey)
		switch lk.State {
		case dns_cache.StateFresh:
			lk.Msg.Id = id
			return lk.Msg, nil
		case dns_cache.StateStale:
			serveStaleOnError.Inc()
			s.Logger.Warn("Upstream упал, отдаём stale из кэша:", name, "Тип:", qtype)
			lk.Msg.Id = id
			return lk.Msg, nil
		}
		return nil, err
	}

	resp.Id = id
	return resp, nil
}

func (s *DnsServer) handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)

	start := time.Now()
	remoteAddr := w.RemoteAddr().String()
	clientIP, _, _ := net.SplitHostPort(remoteAddr)

	// Resolve the client to a store lookup once per request rather than per
	// question — every question in a single DNS message comes from the same
	// transport endpoint.
	lookup, identified := s.Identifier.Identify(identifier.Request{RemoteAddr: remoteAddr})

	for _, q := range r.Question {
		qtype := dns.TypeToString[q.Qtype]
		qname := q.Name

		useFilter := s.Filter(qname)
		if identified && s.Clients.IsExcluded(lookup) {
			useFilter = false
			s.Logger.Debug("Клиент: ", lookup.Kind, ":", lookup.Value, "исключён из фильтрации")
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

func CreateServer(logger Logger, cache Cache, filter func(string2 string) bool, metric Metric, handlers DnsRequestHandlers, ident identifier.Identifier) *DnsServer {
	conf := config.GetConfig()
	return CreateServerWithResolver(logger, cache, filter, metric, handlers, ident, NewDoHResolver(conf.DoHUpstream, conf.DoHBootstrapIPs...))
}

func CreateServerWithResolver(logger Logger, cache Cache, filter func(string2 string) bool, metric Metric, handlers DnsRequestHandlers, ident identifier.Identifier, upstream UpstreamResolver) *DnsServer {
	conf := config.GetConfig()
	s := &DnsServer{
		Logger:     logger,
		Cache:      cache,
		Filter:     filter,
		Upstream:   upstream,
		Metric:     metric,
		Handlers:   handlers,
		Identifier: ident,
		Clients:    store.Get(),
		SWREnabled: conf.CacheSWR,
	}
	// Refresh worker shares the singleflight group with the synchronous hot
	// path, so a refresh that fires while a client miss is in flight (or vice
	// versa) collapses to a single upstream call.
	s.Refresh = newRefreshWorker(cache, upstream, &s.upstream, logger, conf.CacheRefreshConcurrency)
	return s
}
