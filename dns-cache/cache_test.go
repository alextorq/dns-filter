package dns_cache

import (
	"net"
	"testing"
	"time"

	lru_cache "github.com/alextorq/dns-filter/lru-cache"
	"github.com/miekg/dns"
)

// clock is a controllable time source so the tests do not need real
// sleeps to advance cache age.
type clock struct {
	t time.Time
}

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newClock() *clock { return &clock{t: time.Unix(1_700_000_000, 0)} }

// newCacheWithClock builds a Cache whose clock can be advanced from the
// test. Equivalent to NewCache(cap) plus a clock swap.
func newCacheWithClock(c *clock, capacity int) *Cache {
	return &Cache{
		inner:          lru_cache.CreateCache[cachedEntry](capacity),
		negativeTTLCap: DefaultNegativeTTLCap,
		now:            c.now,
	}
}

// newCacheWithSWR builds a Cache with a non-zero stale grace window so the
// SWR-specific tests can exercise the Stale return state.
func newCacheWithSWR(c *clock, capacity int, grace, staleTTL time.Duration) *Cache {
	cache := &Cache{
		inner:          lru_cache.CreateCache[cachedEntry](capacity),
		negativeTTLCap: DefaultNegativeTTLCap,
		now:            c.now,
	}
	cache.staleGrace.Store(int64(grace))
	cache.staleTTL.Store(int64(staleTTL))
	return cache
}

func answerMsg(name string, ttl uint32) *dns.Msg {
	msg := new(dns.Msg)
	msg.Question = []dns.Question{{Name: dns.Fqdn(name), Qtype: dns.TypeA, Qclass: dns.ClassINET}}
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{Name: dns.Fqdn(name), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl},
			A:   net.IPv4(1, 2, 3, 4),
		},
	}
	return msg
}

func soaRR(zone string, hdrTTL, minTTL uint32) *dns.SOA {
	return &dns.SOA{
		Hdr:     dns.RR_Header{Name: dns.Fqdn(zone), Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: hdrTTL},
		Ns:      "ns." + dns.Fqdn(zone),
		Mbox:    "hostmaster." + dns.Fqdn(zone),
		Serial:  1,
		Refresh: 3600,
		Retry:   600,
		Expire:  86400,
		Minttl:  minTTL,
	}
}

func nxdomainMsg(name string, soaHdrTTL, soaMin uint32) *dns.Msg {
	msg := new(dns.Msg)
	msg.Question = []dns.Question{{Name: dns.Fqdn(name), Qtype: dns.TypeA, Qclass: dns.ClassINET}}
	msg.Rcode = dns.RcodeNameError
	msg.Ns = []dns.RR{soaRR("example.com", soaHdrTTL, soaMin)}
	return msg
}

func servfailMsg(name string) *dns.Msg {
	msg := new(dns.Msg)
	msg.Question = []dns.Question{{Name: dns.Fqdn(name), Qtype: dns.TypeA, Qclass: dns.ClassINET}}
	msg.Rcode = dns.RcodeServerFailure
	return msg
}

// Happy path: a hit within the TTL window returns the cached message
// with its TTL decremented; once the TTL has elapsed the next Get is a
// (typed) expired miss.
func TestCache_PositiveTTLHitThenExpire(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)

	cache.Add("k", answerMsg("example.com", 5))

	c.advance(1 * time.Second)
	got := cache.Get("k")
	if !got.Hit {
		t.Fatalf("expected hit at 1s, got %+v", got)
	}
	if ttl := got.Msg.Answer[0].Header().Ttl; ttl != 4 {
		t.Fatalf("expected TTL decremented to 4, got %d", ttl)
	}

	c.advance(5 * time.Second) // total 6s, past the 5s TTL
	got = cache.Get("k")
	if got.Hit || !got.Expired {
		t.Fatalf("expected expired miss at 6s, got %+v", got)
	}

	// Expired entries stay in the LRU to avoid a Get→Delete race with a
	// concurrent Add. A subsequent Get still reports Expired until the
	// caller refreshes the slot via Add.
	got = cache.Get("k")
	if got.Hit || !got.Expired {
		t.Fatalf("expected sticky expired miss until refresh, got %+v", got)
	}

	// Re-Add must refresh the slot in place.
	cache.Add("k", answerMsg("example.com", 5))
	got = cache.Get("k")
	if !got.Hit {
		t.Fatalf("expected hit after refresh, got %+v", got)
	}
}

// Returned messages must be independent copies — mutating one must not
// poison the cache for the next caller.
func TestCache_GetReturnsIndependentCopy(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)
	cache.Add("k", answerMsg("example.com", 60))

	first := cache.Get("k")
	if !first.Hit {
		t.Fatalf("expected first hit")
	}
	first.Msg.Answer[0].Header().Ttl = 999
	first.Msg.Id = 42

	second := cache.Get("k")
	if !second.Hit {
		t.Fatalf("expected second hit")
	}
	if second.Msg.Answer[0].Header().Ttl == 999 {
		t.Fatalf("cache returned a shared pointer; mutation leaked into the cached entry")
	}
	if second.Msg.Id == 42 {
		t.Fatalf("cache returned a shared pointer for Id; mutation leaked")
	}
}

// Caller-side mutation of the *dns.Msg passed to Add must not leak into
// the cache — the cache is required to take its own copy.
func TestCache_AddTakesOwnCopy(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)

	msg := answerMsg("example.com", 60)
	cache.Add("k", msg)
	// Mutate the original after Add — must not affect what is cached.
	msg.Answer[0].(*dns.A).A = net.IPv4(9, 9, 9, 9)
	msg.Answer[0].Header().Ttl = 0

	got := cache.Get("k")
	if !got.Hit {
		t.Fatalf("expected hit")
	}
	if a := got.Msg.Answer[0].(*dns.A).A.To4(); !a.Equal(net.IPv4(1, 2, 3, 4).To4()) {
		t.Fatalf("cache aliased caller's msg, got A=%v", a)
	}
}

// RFC 2308: NXDOMAIN with a sane SOA.Minttl should cache for that
// duration. The SOA header TTL is the upper bound.
func TestCache_NegativeCachingUsesSOAMin(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)

	cache.Add("k", nxdomainMsg("nope.example.com", 3600, 60))
	c.advance(59 * time.Second)
	if !cache.Get("k").Hit {
		t.Fatalf("expected hit at 59s")
	}
	c.advance(2 * time.Second) // total 61s
	got := cache.Get("k")
	if got.Hit || !got.Expired {
		t.Fatalf("expected expired miss at 61s, got %+v", got)
	}
}

// A misbehaving zone returning SOA.Minttl=86400 must not let one bad
// NXDOMAIN stick for a day. The cap clamps it to DefaultNegativeTTLCap.
func TestCache_NegativeCachingCappedAtMax(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)

	cache.Add("k", nxdomainMsg("nope.example.com", 86400, 86400))
	c.advance(DefaultNegativeTTLCap - time.Second)
	if !cache.Get("k").Hit {
		t.Fatalf("expected hit just before cap")
	}
	c.advance(2 * time.Second)
	got := cache.Get("k")
	if got.Hit || !got.Expired {
		t.Fatalf("expected expired miss past cap (%s), got %+v", DefaultNegativeTTLCap, got)
	}
}

// NODATA (Rcode=NOERROR with empty Answer) must also use the SOA TTL,
// not be cached forever on an absent-but-valid answer.
func TestCache_NodataCachedFromSOA(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)

	msg := new(dns.Msg)
	msg.Question = []dns.Question{{Name: "host.example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}}
	msg.Rcode = dns.RcodeSuccess
	msg.Ns = []dns.RR{soaRR("example.com", 7200, 30)}

	res := cache.Add("k", msg)
	if !res.Cached {
		t.Fatalf("expected NODATA to be cached via SOA TTL")
	}
	c.advance(29 * time.Second)
	if !cache.Get("k").Hit {
		t.Fatalf("expected hit at 29s")
	}
	c.advance(2 * time.Second)
	got := cache.Get("k")
	if got.Hit || !got.Expired {
		t.Fatalf("expected expired miss at 31s, got %+v", got)
	}
}

// SERVFAIL must never be cached — a transient upstream blip should not
// pin failure on the next caller.
func TestCache_DoesNotCacheServfail(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)

	res := cache.Add("k", servfailMsg("example.com"))
	if res.Cached {
		t.Fatalf("SERVFAIL must not be cached, got %+v", res)
	}
	if got := cache.Get("k"); got.Hit || got.Expired {
		t.Fatalf("expected plain miss, got %+v", got)
	}
}

// Every Rcode outside the explicitly cacheable set (Success, NXDOMAIN)
// must be refused. SERVFAIL is covered above; this one nails down the
// rest so a future "let's also cache REFUSED" change has to update a
// test.
func TestCache_DoesNotCacheUncacheableRcodes(t *testing.T) {
	cases := []struct {
		name  string
		rcode int
	}{
		{"REFUSED", dns.RcodeRefused},
		{"NOTIMP", dns.RcodeNotImplemented},
		{"FORMERR", dns.RcodeFormatError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newClock()
			cache := newCacheWithClock(c, 10)

			msg := answerMsg("example.com", 60)
			msg.Rcode = tc.rcode

			if cache.Add("k", msg).Cached {
				t.Fatalf("Rcode=%s must not be cached", tc.name)
			}
		})
	}
}

// Truncated (TC=1) responses are incomplete by design — RFC 7766 says
// the client retries over TCP. Caching the truncated copy would pin the
// short-answer version even when the full one is available.
func TestCache_DoesNotCacheTruncated(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)

	msg := answerMsg("example.com", 60)
	msg.Truncated = true
	res := cache.Add("k", msg)
	if res.Cached {
		t.Fatalf("TC=1 must not be cached, got %+v", res)
	}
	if got := cache.Get("k"); got.Hit {
		t.Fatalf("expected miss")
	}
}

// TTL=0 means "do not cache" per RFC 1035; the cache must honour that.
func TestCache_DoesNotCacheZeroTTL(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)

	res := cache.Add("k", answerMsg("example.com", 0))
	if res.Cached {
		t.Fatalf("TTL=0 must not be cached, got %+v", res)
	}
	if got := cache.Get("k"); got.Hit {
		t.Fatalf("expected miss")
	}
}

// A negative answer with no SOA in the Authority section is also not
// cacheable — we have no authoritative TTL to honour.
func TestCache_DoesNotCacheNegativeWithoutSOA(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)

	msg := new(dns.Msg)
	msg.Question = []dns.Question{{Name: "nope.example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}}
	msg.Rcode = dns.RcodeNameError

	if cache.Add("k", msg).Cached {
		t.Fatalf("NXDOMAIN without SOA must not be cached")
	}
}

// minTTL must be the floor across Answer, Authority and Additional. A
// long-lived A record should not mask a short-TTL extra. OPT (used for
// EDNS, with a non-TTL "TTL" field) must be skipped.
func TestCache_MinTTLSpansAllSectionsAndIgnoresOPT(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)

	msg := answerMsg("example.com", 3600)
	msg.Ns = []dns.RR{soaRR("example.com", 7200, 7200)}
	msg.Extra = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{Name: "ns.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30},
			A:   net.IPv4(5, 6, 7, 8),
		},
		// OPT pseudo-RR carries EDNS flags in the Ttl field; if we
		// counted it we would refuse to cache half the internet.
		&dns.OPT{Hdr: dns.RR_Header{Name: ".", Rrtype: dns.TypeOPT, Class: 1232, Ttl: 0}},
	}

	cache.Add("k", msg)
	c.advance(29 * time.Second)
	if !cache.Get("k").Hit {
		t.Fatalf("expected hit at 29s (min TTL is 30 from Extra)")
	}
	c.advance(2 * time.Second) // total 31s
	got := cache.Get("k")
	if got.Hit || !got.Expired {
		t.Fatalf("expected expired miss at 31s, got %+v", got)
	}
}

// Returned RRs must never carry TTL=0 even if the entry is about to
// expire — clients treat 0 as "do not cache", which would defeat
// downstream caching on the very last second of a record's life.
func TestCache_TTLFlooredAtOne(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)

	cache.Add("k", answerMsg("example.com", 2))
	c.advance(2*time.Second - time.Millisecond)
	got := cache.Get("k")
	if !got.Hit {
		t.Fatalf("expected hit just before expiry, got %+v", got)
	}
	if ttl := got.Msg.Answer[0].Header().Ttl; ttl != 1 {
		t.Fatalf("expected TTL floored at 1, got %d", ttl)
	}
}

// Re-Adding an existing key must reset the timer — otherwise a refresh
// after expiry would still be expired.
func TestCache_AddRefreshesEntry(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)
	cache.Add("k", answerMsg("example.com", 10))
	c.advance(9 * time.Second)
	cache.Add("k", answerMsg("example.com", 10)) // refresh
	c.advance(5 * time.Second)                   // 14s since first add, 5s since refresh

	got := cache.Get("k")
	if !got.Hit {
		t.Fatalf("expected hit after refresh, got %+v", got)
	}
	if ttl := got.Msg.Answer[0].Header().Ttl; ttl != 5 {
		t.Fatalf("expected TTL based on refresh time (5), got %d", ttl)
	}
}

// Inside the stale-window — after TTL but before staleUntil — a positive
// answer is served as Stale (not Hit) with RR.Ttl clamped to StaleTTL so the
// client comes back quickly enough for our async refresh to land. This is the
// core SWR guarantee.
func TestCache_StaleHitInGraceWindow(t *testing.T) {
	c := newClock()
	cache := newCacheWithSWR(c, 10, 10*time.Minute, 30*time.Second)

	cache.Add("k", answerMsg("example.com", 60))
	// 1s past expiry — well inside the 10-minute grace window.
	c.advance(61 * time.Second)

	got := cache.Get("k")
	if got.Hit || !got.Stale {
		t.Fatalf("expected Stale=true Hit=false within grace, got %+v", got)
	}
	if got.Msg == nil {
		t.Fatalf("Stale return must carry a message")
	}
	if ttl := got.Msg.Answer[0].Header().Ttl; ttl == 0 || ttl > 30 {
		t.Fatalf("stale TTL must be clamped to StaleTTL=30 and non-zero, got %d", ttl)
	}
}

// Once we pass staleUntil (expiresAt + StaleGrace) the entry is dead: it must
// be reported as Expired, not Stale, so the caller knows to block on upstream.
func TestCache_PastStaleUntilIsExpired(t *testing.T) {
	c := newClock()
	cache := newCacheWithSWR(c, 10, 5*time.Second, 30*time.Second)

	cache.Add("k", answerMsg("example.com", 10))
	// 10s TTL + 5s grace = staleUntil; advance just past it.
	c.advance(16 * time.Second)

	got := cache.Get("k")
	if got.Hit || got.Stale {
		t.Fatalf("expected Expired after staleUntil, got %+v", got)
	}
	if !got.Expired {
		t.Fatalf("expected Expired=true, got %+v", got)
	}
}

// Negative responses (NXDOMAIN, NODATA) must never enter the stale-window
// even when staleGrace > 0. Serving stale "does not exist" past TTL would
// pin a wrong negative answer for a domain that was just registered.
func TestCache_NegativeResponsesNeverGoStale(t *testing.T) {
	cases := []struct {
		name string
		msg  *dns.Msg
	}{
		{
			name: "NXDOMAIN",
			msg:  nxdomainMsg("nope.example.com", 3600, 60),
		},
		{
			name: "NODATA",
			msg: func() *dns.Msg {
				m := new(dns.Msg)
				m.Question = []dns.Question{{Name: "host.example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}}
				m.Rcode = dns.RcodeSuccess
				m.Ns = []dns.RR{soaRR("example.com", 7200, 60)}
				return m
			}(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newClock()
			cache := newCacheWithSWR(c, 10, 24*time.Hour, 30*time.Second)

			cache.Add("k", tc.msg)
			// Past TTL (60s) — under a fresh cache this would be inside the
			// 24h grace window for a positive answer. For a negative one it
			// must be Expired, not Stale.
			c.advance(61 * time.Second)
			got := cache.Get("k")
			if got.Stale {
				t.Fatalf("negative answer must not be served stale: %+v", got)
			}
			if !got.Expired {
				t.Fatalf("expected Expired for %s past TTL, got %+v", tc.name, got)
			}
		})
	}
}

// Clear must wipe every entry, leave the cache usable, and report the count
// it removed so the admin endpoint can echo it back to the operator.
func TestCache_ClearWipesAllEntries(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)

	cache.Add("a", answerMsg("a.example.com", 60))
	cache.Add("b", answerMsg("b.example.com", 60))

	if n := cache.Clear(); n != 2 {
		t.Fatalf("expected Clear to return 2, got %d", n)
	}
	if got := cache.Get("a"); got.Hit || got.Stale || got.Expired {
		t.Fatalf("expected plain miss after Clear, got %+v", got)
	}
	if l := cache.Len(); l != 0 {
		t.Fatalf("expected Len 0 after Clear, got %d", l)
	}

	// Cache must remain usable: a fresh Add after Clear behaves like a cold
	// insert. Without this we could silently break the global singleton.
	cache.Add("c", answerMsg("c.example.com", 60))
	if got := cache.Get("c"); !got.Hit {
		t.Fatalf("cache unusable after Clear, expected hit, got %+v", got)
	}
}

// Edge case: Clear on a cold cache must be a 0-returning no-op so the
// admin endpoint reports "nothing to clear" instead of misleading numbers.
func TestCache_ClearEmptyReturnsZero(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10)
	if n := cache.Clear(); n != 0 {
		t.Fatalf("expected 0 on empty Clear, got %d", n)
	}
}

// staleGrace=0 must be a true no-op: behaviour stays bit-for-bit identical
// to the pre-SWR cache. Locks in the back-compat guarantee for embedders.
func TestCache_ZeroGraceDisablesStale(t *testing.T) {
	c := newClock()
	cache := newCacheWithSWR(c, 10, 0, 30*time.Second)

	cache.Add("k", answerMsg("example.com", 5))
	c.advance(6 * time.Second)

	got := cache.Get("k")
	if got.Stale {
		t.Fatalf("staleGrace=0 must never produce Stale, got %+v", got)
	}
	if !got.Expired {
		t.Fatalf("expected Expired with staleGrace=0, got %+v", got)
	}
}

// SetStaleGrace must take effect for entries cached after the change: a cache
// that started with no stale-window begins serving Stale once grace is raised
// at runtime (the settings-driven path).
func TestCache_SetStaleGrace_EnablesStaleForNewEntries(t *testing.T) {
	c := newClock()
	cache := newCacheWithClock(c, 10) // grace 0 by default
	cache.SetStaleTTL(30 * time.Second)

	cache.SetStaleGrace(time.Hour)
	cache.Add("k", answerMsg("example.com", 5))
	c.advance(6 * time.Second) // past the 5s TTL, inside the 1h grace

	if got := cache.Get("k"); !got.Stale {
		t.Fatalf("expected Stale after raising grace at runtime, got %+v", got)
	}
}

// Conversely, lowering grace back to 0 stops new entries from being served
// stale — the runtime toggle is reversible.
func TestCache_SetStaleGrace_ZeroDisablesStaleForNewEntries(t *testing.T) {
	c := newClock()
	cache := newCacheWithSWR(c, 10, time.Hour, 30*time.Second)

	cache.SetStaleGrace(0)
	cache.Add("k", answerMsg("example.com", 5))
	c.advance(6 * time.Second)

	if got := cache.Get("k"); got.Stale {
		t.Fatalf("expected no Stale after lowering grace to 0, got %+v", got)
	}
}
