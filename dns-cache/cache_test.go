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
