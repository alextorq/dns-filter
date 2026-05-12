package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alextorq/dns-filter/clients/identifier"
	dnsLib "github.com/miekg/dns"
)

type noopLogger struct{}

func (noopLogger) Info(args ...interface{})  {}
func (noopLogger) Error(err error)           {}
func (noopLogger) Debug(args ...interface{}) {}
func (noopLogger) Warn(args ...interface{})  {}

type memoryCache struct {
	mu     sync.Mutex
	values map[string]*dnsLib.Msg
}

func (c *memoryCache) Get(key string) (*dnsLib.Msg, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.values[key]
	return v, ok
}

func (c *memoryCache) Add(key string, val *dnsLib.Msg) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = val
}

type staticResolver struct {
	calls atomic.Int64
	err   error
	rcode int
}

func (r *staticResolver) Exchange(_ context.Context, msg *dnsLib.Msg) (*dnsLib.Msg, error) {
	r.calls.Add(1)
	if r.err != nil {
		return nil, r.err
	}

	reply := new(dnsLib.Msg)
	reply.SetReply(msg)
	reply.Rcode = r.rcode
	return reply, nil
}

// blockingResolver counts calls and blocks every Exchange until release is
// closed. Used to force concurrent callers to pile up on the same in-flight
// upstream request so we can verify singleflight coalescing.
type blockingResolver struct {
	calls   atomic.Int64
	release chan struct{}
	err     error
	rcode   int
}

func (r *blockingResolver) Exchange(_ context.Context, msg *dnsLib.Msg) (*dnsLib.Msg, error) {
	r.calls.Add(1)
	<-r.release
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

type noopClientStore struct{}

func (noopClientStore) IsExcluded(_ identifier.Lookup) bool { return false }

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
	if got := resolver.calls.Load(); got != 1 {
		t.Fatalf("expected one upstream call, got %d", got)
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
		Identifier:        identifier.IPIdentifier{},
		Clients:           noopClientStore{},
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
		Logger:     noopLogger{},
		Cache:      &memoryCache{values: map[string]*dnsLib.Msg{}},
		Filter:     func(string) bool { return false },
		Upstream:   &staticResolver{rcode: dnsLib.RcodeNameError},
		Metric:     noopMetric{},
		Handlers:   noopHandlers{},
		Identifier: identifier.IPIdentifier{},
		Clients:    noopClientStore{},
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

// Concurrent identical queries on a cold cache must collapse into exactly one
// upstream call. Without singleflight, N callers race past the cache miss and
// each fires its own DoH request — the very thundering-herd this PR fixes.
func TestGetFromCacheOrCreateRequestCoalescesConcurrentCalls(t *testing.T) {
	release := make(chan struct{})
	resolver := &blockingResolver{release: release, rcode: dnsLib.RcodeSuccess}
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

	const N = 50
	results := make([]*dnsLib.Msg, N)
	errs := make([]error, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := range N {
		go func(i int) {
			defer wg.Done()
			r, err := server.GetFromCacheOrCreateRequest(context.Background(), question, uint16(i+1))
			results[i] = r
			errs[i] = err
		}(i)
	}

	// Give callers a moment to pile up on the singleflight key before the
	// upstream is allowed to return. The blocking resolver guarantees the
	// first goroutine cannot complete fn() until we close release, so any
	// goroutine that enters Do during this window will coalesce.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := resolver.calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 upstream call after coalescing, got %d", got)
	}
	for i, r := range results {
		if errs[i] != nil {
			t.Fatalf("caller %d returned error: %v", i, errs[i])
		}
		if r == nil {
			t.Fatalf("caller %d got nil response", i)
		}
		// Each caller must see its own Id even though the underlying upstream
		// reply is shared — otherwise the response would be racy.
		if r.Id != uint16(i+1) {
			t.Fatalf("caller %d: expected id %d, got %d", i, i+1, r.Id)
		}
	}
	// Shared callers must receive distinct *dns.Msg pointers, otherwise the
	// per-caller `msg.Id = id` mutation in the hot path would be a data race
	// on the same object. Counting unique pointers across all N callers locks
	// in the Copy()-on-shared contract in upstreamCoordinator.Do.
	seen := make(map[*dnsLib.Msg]struct{}, N)
	for _, r := range results {
		seen[r] = struct{}{}
	}
	if len(seen) != N {
		t.Fatalf("expected %d distinct response objects, got %d (msg.Copy missing on shared return)", N, len(seen))
	}
}

// Distinct cache keys must not coalesce. This protects against a coordinator
// that over-collapses (e.g. keys off only the name and ignores qtype).
func TestGetFromCacheOrCreateRequestDoesNotCoalesceDistinctKeys(t *testing.T) {
	resolver := &staticResolver{rcode: dnsLib.RcodeSuccess}
	server := &DnsServer{
		Logger:   noopLogger{},
		Cache:    &memoryCache{values: map[string]*dnsLib.Msg{}},
		Upstream: resolver,
	}

	const N = 10
	var wg sync.WaitGroup
	wg.Add(N)
	for i := range N {
		go func(i int) {
			defer wg.Done()
			q := dnsLib.Question{
				Name:   fmt.Sprintf("example%d.com.", i),
				Qtype:  dnsLib.TypeA,
				Qclass: dnsLib.ClassINET,
			}
			if _, err := server.GetFromCacheOrCreateRequest(context.Background(), q, uint16(i+1)); err != nil {
				t.Errorf("caller %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	if got := resolver.calls.Load(); got != int64(N) {
		t.Fatalf("expected %d upstream calls for distinct keys, got %d", N, got)
	}
}

// Coalesced upstream errors must propagate to every waiter; the first caller
// failing should not leave the others hung or hiding behind a stale nil.
func TestGetFromCacheOrCreateRequestCoalescesUpstreamErrors(t *testing.T) {
	release := make(chan struct{})
	wantErr := errors.New("upstream down")
	resolver := &blockingResolver{release: release, err: wantErr}
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

	const N = 20
	errs := make([]error, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := range N {
		go func(i int) {
			defer wg.Done()
			_, err := server.GetFromCacheOrCreateRequest(context.Background(), question, uint16(i+1))
			errs[i] = err
		}(i)
	}

	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	for i, err := range errs {
		if !errors.Is(err, wantErr) {
			t.Fatalf("caller %d: expected %v, got %v", i, wantErr, err)
		}
	}
}
