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
	dns_cache "github.com/alextorq/dns-filter/dns-cache"
	dnsLib "github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type noopLogger struct{}

func (noopLogger) Info(args ...interface{})  {}
func (noopLogger) Error(err error)           {}
func (noopLogger) Debug(args ...interface{}) {}
func (noopLogger) Warn(args ...interface{})  {}

// memoryCache is a controllable in-memory stand-in for the production
// dns_cache.CacheWithMetrics. Tests can pre-seed Stale entries via addStale
// to exercise SWR paths without dealing with real clocks/TTL.
type memoryCache struct {
	mu     sync.Mutex
	values map[string]memoryCacheEntry
}

type memoryCacheEntry struct {
	msg   *dnsLib.Msg
	state dns_cache.State
}

func newMemoryCache() *memoryCache {
	return &memoryCache{values: map[string]memoryCacheEntry{}}
}

func (c *memoryCache) Lookup(key string) dns_cache.Lookup {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.values[key]
	if !ok {
		return dns_cache.Lookup{State: dns_cache.StateMiss}
	}
	// Cache returns owned copies so caller-side mutation (e.g. msg.Id = id)
	// does not leak back into our store.
	return dns_cache.Lookup{Msg: e.msg.Copy(), State: e.state}
}

func (c *memoryCache) Add(key string, val *dnsLib.Msg) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = memoryCacheEntry{msg: val.Copy(), state: dns_cache.StateFresh}
}

// addStale seeds a Stale entry directly so a test can exercise the SWR path
// without setting up a real clock-driven cache.
func (c *memoryCache) addStale(key string, val *dnsLib.Msg) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = memoryCacheEntry{msg: val.Copy(), state: dns_cache.StateStale}
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
		Cache:    newMemoryCache(),
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
		Cache:    newMemoryCache(),
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
		Cache:             newMemoryCache(),
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
		Cache:      newMemoryCache(),
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
		Cache:    newMemoryCache(),
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
		Cache:    newMemoryCache(),
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
		Cache:    newMemoryCache(),
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

// eventuallyTrue polls pred every 2ms until it returns true or the deadline
// elapses; on timeout it fails the test. Used instead of fixed sleeps to keep
// SWR tests deterministic on a slow CI runner.
func eventuallyTrue(t *testing.T, timeout time.Duration, pred func() bool, label string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition %q not met within %s", label, timeout)
}

func newAnswerMsg(name string, ttl uint32) *dnsLib.Msg {
	msg := new(dnsLib.Msg)
	msg.Question = []dnsLib.Question{{Name: dnsLib.Fqdn(name), Qtype: dnsLib.TypeA, Qclass: dnsLib.ClassINET}}
	msg.Rcode = dnsLib.RcodeSuccess
	msg.Answer = []dnsLib.RR{
		&dnsLib.A{
			Hdr: dnsLib.RR_Header{Name: dnsLib.Fqdn(name), Rrtype: dnsLib.TypeA, Class: dnsLib.ClassINET, Ttl: ttl},
			A:   net.IPv4(1, 2, 3, 4),
		},
	}
	return msg
}

// Happy path for SWR: with SWREnabled=true a Stale cache lookup returns the
// cached payload to the client immediately AND triggers a background refresh
// that re-populates the cache with a Fresh entry. Verifies both invariants
// without relying on fixed sleeps.
func TestGetFromCacheOrCreateRequest_SWRStaleHitTriggersAsyncRefresh(t *testing.T) {
	resolver := &staticResolver{rcode: dnsLib.RcodeSuccess}
	cache := newMemoryCache()
	cache.addStale("example.com.:A", newAnswerMsg("example.com", 60))

	server := &DnsServer{
		Logger:   noopLogger{},
		Cache:    cache,
		Upstream: resolver,
	}
	server.SetSWR(true)
	server.Refresh = newRefreshWorker(cache, resolver, &server.upstream, noopLogger{}, 4)

	question := dnsLib.Question{Name: "example.com.", Qtype: dnsLib.TypeA, Qclass: dnsLib.ClassINET}

	resp, err := server.GetFromCacheOrCreateRequest(context.Background(), question, 99)
	if err != nil {
		t.Fatalf("expected stale response without error, got %v", err)
	}
	if resp == nil {
		t.Fatal("expected stale response")
	}
	if resp.Id != 99 {
		t.Fatalf("expected Id=99 on stale response, got %d", resp.Id)
	}

	// Refresh runs in the background — wait until it has called upstream and
	// the cache transitions Stale → Fresh.
	eventuallyTrue(t, time.Second, func() bool {
		return resolver.calls.Load() >= 1
	}, "background refresh fires upstream call")
	eventuallyTrue(t, time.Second, func() bool {
		return cache.Lookup("example.com.:A").State == dns_cache.StateFresh
	}, "cache returns to Fresh after refresh")
}

// SWR disabled: a Stale cache hit must NOT short-circuit to stale. The
// resolver should fall through to the synchronous upstream path so callers
// get a freshly fetched answer (the cache is also updated as a side effect).
func TestGetFromCacheOrCreateRequest_SWRDisabledStaleFallsThroughToUpstream(t *testing.T) {
	resolver := &staticResolver{rcode: dnsLib.RcodeSuccess}
	cache := newMemoryCache()
	cache.addStale("example.com.:A", newAnswerMsg("example.com", 60))

	server := &DnsServer{
		Logger:   noopLogger{},
		Cache:    cache,
		Upstream: resolver,
	}
	server.SetSWR(false) // disabled

	question := dnsLib.Question{Name: "example.com.", Qtype: dnsLib.TypeA, Qclass: dnsLib.ClassINET}

	resp, err := server.GetFromCacheOrCreateRequest(context.Background(), question, 50)
	if err != nil {
		t.Fatalf("expected successful upstream call, got %v", err)
	}
	if resp == nil {
		t.Fatal("expected response from upstream fallthrough")
	}
	if resp.Id != 50 {
		t.Fatalf("expected Id=50, got %d", resp.Id)
	}
	if got := resolver.calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 synchronous upstream call when SWR is off, got %d", got)
	}
}

// Serve-stale-on-error (RFC 8767): even with SWR disabled, when the upstream
// call fails AND a stale entry exists, we must hand the stale answer back to
// the client instead of returning SERVFAIL. This is the resilience guarantee
// and is always on regardless of SWREnabled.
func TestGetFromCacheOrCreateRequest_ServeStaleOnError(t *testing.T) {
	wantErr := errors.New("upstream down")
	resolver := &staticResolver{err: wantErr}
	cache := newMemoryCache()
	cache.addStale("example.com.:A", newAnswerMsg("example.com", 60))

	server := &DnsServer{
		Logger:   noopLogger{},
		Cache:    cache,
		Upstream: resolver,
	}
	server.SetSWR(false) // off so we exercise the synchronous-error → stale-fallback path

	question := dnsLib.Question{Name: "example.com.", Qtype: dnsLib.TypeA, Qclass: dnsLib.ClassINET}

	resp, err := server.GetFromCacheOrCreateRequest(context.Background(), question, 11)
	if err != nil {
		t.Fatalf("expected stale fallback to mask upstream error, got err: %v", err)
	}
	if resp == nil {
		t.Fatal("expected stale response after upstream failure")
	}
	if resp.Id != 11 {
		t.Fatalf("expected Id=11 on stale-on-error response, got %d", resp.Id)
	}
	if got := resolver.calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 upstream attempt before falling back to stale, got %d", got)
	}
}

// When the refresh semaphore is saturated, additional stale hits must drop
// their refresh attempt (counted) but still serve stale to the client. This
// is the back-pressure guarantee that keeps the goroutine count bounded.
func TestGetFromCacheOrCreateRequest_RefreshDroppedWhenSemaphoreFull(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	resolver := &blockingResolver{release: release, rcode: dnsLib.RcodeSuccess}
	cache := newMemoryCache()
	cache.addStale("a.example.com.:A", newAnswerMsg("a.example.com", 60))
	cache.addStale("b.example.com.:A", newAnswerMsg("b.example.com", 60))

	server := &DnsServer{
		Logger:   noopLogger{},
		Cache:    cache,
		Upstream: resolver,
	}
	server.SetSWR(true)
	// concurrency=1 so a second refresh has nowhere to land.
	server.Refresh = newRefreshWorker(cache, resolver, &server.upstream, noopLogger{}, 1)

	droppedBefore := testutil.ToFloat64(refreshTotal.WithLabelValues("dropped"))

	// First stale-hit: refresh acquires the only semaphore slot and blocks
	// inside Exchange() until we close(release).
	qA := dnsLib.Question{Name: "a.example.com.", Qtype: dnsLib.TypeA, Qclass: dnsLib.ClassINET}
	if _, err := server.GetFromCacheOrCreateRequest(context.Background(), qA, 1); err != nil {
		t.Fatalf("first stale hit: %v", err)
	}
	eventuallyTrue(t, time.Second, func() bool {
		return resolver.calls.Load() >= 1
	}, "first refresh enters Exchange and holds the semaphore")

	// Second stale-hit on a different key: semaphore is held, so its refresh
	// must be dropped instead of spawning another goroutine. The client still
	// gets the stale answer.
	qB := dnsLib.Question{Name: "b.example.com.", Qtype: dnsLib.TypeA, Qclass: dnsLib.ClassINET}
	resp, err := server.GetFromCacheOrCreateRequest(context.Background(), qB, 2)
	if err != nil {
		t.Fatalf("second stale hit returned error: %v", err)
	}
	if resp == nil || resp.Id != 2 {
		t.Fatalf("expected stale response for B with Id=2, got %+v", resp)
	}

	eventuallyTrue(t, time.Second, func() bool {
		return testutil.ToFloat64(refreshTotal.WithLabelValues("dropped"))-droppedBefore >= 1
	}, "dropped counter increments when semaphore is full")
}

// Race window between our miss and our upstream error: another caller (or a
// proactive refresh) may have populated the cache to Fresh by the time we
// fall through to the error-recovery Lookup. In that case we must serve the
// Fresh answer instead of SERVFAILing on top of it.
func TestGetFromCacheOrCreateRequest_UpstreamErrorPrefersConcurrentFresh(t *testing.T) {
	release := make(chan struct{})
	wantErr := errors.New("upstream down")
	resolver := &blockingResolver{release: release, err: wantErr}
	cache := newMemoryCache()
	server := &DnsServer{
		Logger:   noopLogger{},
		Cache:    cache,
		Upstream: resolver,
	}

	question := dnsLib.Question{Name: "example.com.", Qtype: dnsLib.TypeA, Qclass: dnsLib.ClassINET}

	type result struct {
		resp *dnsLib.Msg
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		r, err := server.GetFromCacheOrCreateRequest(context.Background(), question, 42)
		resultCh <- result{r, err}
	}()

	// Block until the upstream call is in flight so we know our caller is
	// past the initial Lookup and committed to the upstream branch.
	eventuallyTrue(t, time.Second, func() bool {
		return resolver.calls.Load() >= 1
	}, "upstream call enters Exchange before the concurrent Fresh write")

	// Simulate a concurrent populator (another client or a background refresh)
	// landing a Fresh entry while our upstream call is still blocked.
	cache.Add("example.com.:A", newAnswerMsg("example.com", 60))

	// Release the upstream → returns our wanted error.
	close(release)

	res := <-resultCh
	if res.err != nil {
		t.Fatalf("expected Fresh fallback to mask upstream error, got %v", res.err)
	}
	if res.resp == nil {
		t.Fatal("expected Fresh response after upstream failure")
	}
	if res.resp.Id != 42 {
		t.Fatalf("expected Id=42 from Fresh fallback, got %d", res.resp.Id)
	}
}

// Without a stale entry, an upstream error must still propagate as a normal
// error — serve-stale-on-error must not paper over genuine cold-cache
// failures. Pair to the positive test above.
func TestGetFromCacheOrCreateRequest_UpstreamErrorWithoutStalePropagates(t *testing.T) {
	wantErr := errors.New("upstream down")
	resolver := &staticResolver{err: wantErr}
	server := &DnsServer{
		Logger:   noopLogger{},
		Cache:    newMemoryCache(),
		Upstream: resolver,
	}
	question := dnsLib.Question{Name: "example.com.", Qtype: dnsLib.TypeA, Qclass: dnsLib.ClassINET}

	_, err := server.GetFromCacheOrCreateRequest(context.Background(), question, 1)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected upstream error to propagate (no stale to fall back to), got %v", err)
	}
}
