package dns_cache

import (
	"time"

	lru_cache "github.com/alextorq/dns-filter/lru-cache"
	"github.com/miekg/dns"
)

// DefaultNegativeTTLCap caps how long a negative response (NXDOMAIN /
// NODATA) is honoured even if the authority SOA advertises a much larger
// SOA.Minttl. Without a cap a single misbehaving zone could pin a wrong
// answer for hours; 300s matches what most public resolvers use.
const DefaultNegativeTTLCap = 300 * time.Second

// cachedEntry is what we store inside the LRU. We keep an owned copy of
// the upstream message so callers cannot mutate the cache through the
// shared pointer, and we remember when the entry was cached so we can
// (a) expire it once minTTL has elapsed and (b) decrement RR.Ttl on the
// way out per RFC 1035 §3.2.1.
type cachedEntry struct {
	msg       *dns.Msg
	cachedAt  time.Time
	expiresAt time.Time
}

// Cache is a TTL-aware DNS response cache built on the generic LRU. It is
// not exported as a global — callers wrap it for metrics (see metric.go)
// and the wrapper is the singleton.
type Cache struct {
	inner          *lru_cache.LRUCache[cachedEntry]
	negativeTTLCap time.Duration
	now            func() time.Time
}

func NewCache(capacity int) *Cache {
	return &Cache{
		inner:          lru_cache.CreateCache[cachedEntry](capacity),
		negativeTTLCap: DefaultNegativeTTLCap,
		now:            time.Now,
	}
}

type AddResult struct {
	Cached  bool
	Evicted bool
	Size    int
}

type GetResult struct {
	Msg     *dns.Msg
	Hit     bool
	Expired bool
}

// Add stores a deep copy of msg if it is cacheable. Returns Cached=false
// for responses we deliberately refuse to cache (SERVFAIL, missing TTL,
// negative response without SOA, …) so the metric layer can keep its
// counters honest.
func (c *Cache) Add(key string, msg *dns.Msg) AddResult {
	ttl, ok := computeCacheTTL(msg, c.negativeTTLCap)
	if !ok {
		return AddResult{}
	}
	now := c.now()
	res := c.inner.Add(key, cachedEntry{
		msg:       msg.Copy(),
		cachedAt:  now,
		expiresAt: now.Add(ttl),
	})
	return AddResult{Cached: true, Evicted: res.Evicted, Size: res.Size}
}

// Get returns a fresh, fully-owned copy of the cached message with every
// RR.Ttl decremented by the time it has spent in the cache. An expired
// entry is reported as Expired (not Hit) and left in place: the caller
// will Add a fresh upstream answer, which the LRU updates in-place under
// the same key, so the slot is not lost. Deleting here would race with
// a concurrent Add for the same key — a parallel goroutine could refresh
// the entry between our Get and Delete, and we'd then evict the freshly
// cached value.
func (c *Cache) Get(key string) GetResult {
	entry, ok := c.inner.Get(key)
	if !ok {
		return GetResult{}
	}
	now := c.now()
	if !now.Before(entry.expiresAt) {
		return GetResult{Expired: true}
	}
	msg := entry.msg.Copy()
	decrementTTL(msg, now.Sub(entry.cachedAt))
	return GetResult{Msg: msg, Hit: true}
}

// computeCacheTTL implements the cacheability rules:
//   - Truncated (TC=1) → not cached: the message is incomplete and the
//     client is expected to retry over TCP (RFC 7766).
//   - SERVFAIL / REFUSED / etc. → not cached at all.
//   - NOERROR with answers → min(TTL) across Answer/Authority/Additional,
//     skipping the OPT pseudo-RR whose "TTL" field is reused for EDNS
//     flags (RFC 6891 §6.1.3). TTL=0 means "do not cache" (RFC 1035).
//   - NXDOMAIN or NODATA (NOERROR with empty Answer) → SOA-derived
//     negative TTL per RFC 2308, capped to negCap.
func computeCacheTTL(msg *dns.Msg, negCap time.Duration) (time.Duration, bool) {
	if msg.Truncated {
		return 0, false
	}
	switch msg.Rcode {
	case dns.RcodeSuccess:
		if len(msg.Answer) == 0 {
			return negativeTTL(msg, negCap)
		}
		return positiveTTL(msg)
	case dns.RcodeNameError:
		return negativeTTL(msg, negCap)
	default:
		return 0, false
	}
}

func positiveTTL(msg *dns.Msg) (time.Duration, bool) {
	var minTTL uint32
	found := false
	for _, section := range [][]dns.RR{msg.Answer, msg.Ns, msg.Extra} {
		for _, rr := range section {
			if rr.Header().Rrtype == dns.TypeOPT {
				continue
			}
			ttl := rr.Header().Ttl
			if !found || ttl < minTTL {
				minTTL = ttl
				found = true
			}
		}
	}
	if !found || minTTL == 0 {
		return 0, false
	}
	return time.Duration(minTTL) * time.Second, true
}

func negativeTTL(msg *dns.Msg, cap time.Duration) (time.Duration, bool) {
	for _, rr := range msg.Ns {
		soa, ok := rr.(*dns.SOA)
		if !ok {
			continue
		}
		// RFC 2308 §5: negative TTL is MIN(SOA.MINIMUM, SOA TTL).
		ttl := min(soa.Minttl, soa.Hdr.Ttl)
		if ttl == 0 {
			return 0, false
		}
		d := min(time.Duration(ttl)*time.Second, cap)
		return d, true
	}
	return 0, false
}

// decrementTTL reduces every RR.Ttl by `elapsed` seconds, flooring at 1
// so clients never see a zero TTL (which they interpret as "do not
// cache"). The OPT pseudo-RR is skipped because its Ttl field is not a
// timer.
func decrementTTL(msg *dns.Msg, elapsed time.Duration) {
	sec := uint32(elapsed / time.Second)
	if sec == 0 {
		return
	}
	for _, section := range [][]dns.RR{msg.Answer, msg.Ns, msg.Extra} {
		for _, rr := range section {
			hdr := rr.Header()
			if hdr.Rrtype == dns.TypeOPT {
				continue
			}
			if hdr.Ttl <= sec {
				hdr.Ttl = 1
			} else {
				hdr.Ttl -= sec
			}
		}
	}
}
