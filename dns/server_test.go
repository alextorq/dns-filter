package dns

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	dnsLib "github.com/miekg/dns"
)

type noopLogger struct{}

func (noopLogger) Info(args ...interface{})  {}
func (noopLogger) Error(err error)           {}
func (noopLogger) Debug(args ...interface{}) {}
func (noopLogger) Warn(args ...interface{})  {}

type memoryCache struct {
	values map[string]*dnsLib.Msg
}

func (c *memoryCache) Get(key string) (*dnsLib.Msg, bool) {
	v, ok := c.values[key]
	return v, ok
}

func (c *memoryCache) Add(key string, val *dnsLib.Msg) {
	c.values[key] = val
}

type staticResolver struct {
	calls int
	err   error
	rcode int
}

func (r *staticResolver) Exchange(_ context.Context, msg *dnsLib.Msg) (*dnsLib.Msg, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}

	reply := new(dnsLib.Msg)
	reply.SetReply(msg)
	reply.Rcode = r.rcode
	return reply, nil
}

type noopMetric struct{}

func (noopMetric) HandleDNSRequest(_ string, _ string, _ string, _ int, _ time.Duration) {}

type noopHandlers struct{}

func (noopHandlers) Allowed(_ dnsLib.ResponseWriter, _ *dnsLib.Msg) {}
func (noopHandlers) Blocked(_ dnsLib.ResponseWriter, _ *dnsLib.Msg) {}

type captureResponseWriter struct {
	msg    *dnsLib.Msg
	local  net.Addr
	remote net.Addr
}

func (w *captureResponseWriter) LocalAddr() net.Addr {
	return w.local
}

func (w *captureResponseWriter) RemoteAddr() net.Addr {
	return w.remote
}

func (w *captureResponseWriter) WriteMsg(msg *dnsLib.Msg) error {
	w.msg = msg.Copy()
	return nil
}

func (w *captureResponseWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func (w *captureResponseWriter) Close() error {
	return nil
}

func (w *captureResponseWriter) TsigStatus() error {
	return nil
}

func (w *captureResponseWriter) TsigTimersOnly(_ bool) {}

func (w *captureResponseWriter) Hijack() {}

func TestGetFromCacheOrCreateRequestUsesDoHResolver(t *testing.T) {
	resolver := &staticResolver{}
	server := &DnsServer{
		Logger:   noopLogger{},
		Cache:    &memoryCache{values: map[string]*dnsLib.Msg{}},
		Upstream: resolver,
	}

	question := dnsLib.Question{
		Name:   "example.com.",
		Qtype:  dnsLib.TypeA,
		Qclass: dnsLib.ClassINET,
	}

	resp, err := server.GetFromCacheOrCreateRequest(context.Background(), question, 100)
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	if resp.Id != 100 {
		t.Fatalf("expected first response id 100, got %d", resp.Id)
	}

	resp, err = server.GetFromCacheOrCreateRequest(context.Background(), question, 200)
	if err != nil {
		t.Fatalf("cached request: %v", err)
	}
	if resp.Id != 200 {
		t.Fatalf("expected cached response id 200, got %d", resp.Id)
	}
	if resolver.calls != 1 {
		t.Fatalf("expected one upstream call, got %d", resolver.calls)
	}
}

func TestGetFromCacheOrCreateRequestReturnsResolverError(t *testing.T) {
	wantErr := errors.New("upstream down")
	server := &DnsServer{
		Logger:   noopLogger{},
		Cache:    &memoryCache{values: map[string]*dnsLib.Msg{}},
		Upstream: &staticResolver{err: wantErr},
	}

	question := dnsLib.Question{
		Name:   "example.com.",
		Qtype:  dnsLib.TypeA,
		Qclass: dnsLib.ClassINET,
	}

	_, err := server.GetFromCacheOrCreateRequest(context.Background(), question, 100)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

// Locks in #33: clients that retry over TCP (RFC 7766 MUST) must get an
// answer from the server, not a connection error. Pre-binds UDP+TCP on an
// ephemeral port so the test does not need root and never races on :53.
func TestServeAnswersOverTCP(t *testing.T) {
	udpAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	port := udpConn.LocalAddr().(*net.UDPAddr).Port
	tcpListener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		udpConn.Close()
		t.Fatalf("listen tcp: %v", err)
	}

	started := make(chan struct{}, 2)
	server := &DnsServer{
		Logger:            noopLogger{},
		Cache:             &memoryCache{values: map[string]*dnsLib.Msg{}},
		Filter:            func(string) bool { return false },
		Upstream:          &staticResolver{rcode: dnsLib.RcodeSuccess},
		Metric:            noopMetric{},
		Handlers:          noopHandlers{},
		NotifyStartedFunc: func() { started <- struct{}{} },
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ServeWithListeners(udpConn, tcpListener) }()
	t.Cleanup(func() {
		_ = server.Shutdown()
		<-serveErr
	})

	// Wait for both UDP and TCP listeners to be ready.
	for range 2 {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for listeners to start")
		}
	}

	addr := tcpListener.Addr().String()
	client := &dnsLib.Client{Net: "tcp", Timeout: 2 * time.Second}
	req := new(dnsLib.Msg)
	req.SetQuestion("example.com.", dnsLib.TypeA)
	resp, _, err := client.Exchange(req, addr)
	if err != nil {
		t.Fatalf("tcp exchange failed: %v", err)
	}
	if resp.Rcode != dnsLib.RcodeSuccess {
		t.Fatalf("expected NOERROR over TCP, got %s", dnsLib.RcodeToString[resp.Rcode])
	}
}

func TestHandleDNSCopiesUpstreamRcode(t *testing.T) {
	server := &DnsServer{
		Logger:   noopLogger{},
		Cache:    &memoryCache{values: map[string]*dnsLib.Msg{}},
		Filter:   func(string) bool { return false },
		Upstream: &staticResolver{rcode: dnsLib.RcodeNameError},
		Metric:   noopMetric{},
		Handlers: noopHandlers{},
	}
	writer := &captureResponseWriter{
		local:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53},
		remote: &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 50000},
	}
	req := new(dnsLib.Msg)
	req.SetQuestion("missing.example.", dnsLib.TypeA)
	req.Id = 77

	server.handleDNS(writer, req)

	if writer.msg == nil {
		t.Fatal("expected DNS response")
	}
	if writer.msg.Rcode != dnsLib.RcodeNameError {
		t.Fatalf("expected NXDOMAIN rcode, got %s", dnsLib.RcodeToString[writer.msg.Rcode])
	}
}
