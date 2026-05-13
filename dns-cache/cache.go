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
//
// staleUntil >= expiresAt is the boundary past which the entry is fully
// dead; in the window (expiresAt, staleUntil] the entry is served as Stale
// (SWR / RFC 8767). For negative responses staleUntil == expiresAt so they
// are never served stale.
type cachedEntry struct {
	msg        *dns.Msg
	cachedAt   time.Time
	expiresAt  time.Time
	staleUntil time.Time
}

// Cache is a TTL-aware DNS response cache built on the generic LRU. It is
// not exported as a global — callers wrap it for metrics (see metric.go)
// and the wrapper is the singleton.
//
// staleGrace=0 disables the stale-window entirely (Get never returns Stale)
// — this is the back-compat default used by NewCache. staleTTL is the value
// written to RR.Ttl when a Stale entry is served; clients respecting it will
// come back soon enough for our async refresh to have landed (RFC 8767 §6
// recommends ≤ 30s).
type Cache struct {
	inner          *lru_cache.LRUCache[cachedEntry]
	negativeTTLCap time.Duration
	staleGrace     time.Duration
	staleTTL       time.Duration
	now            func() time.Time
}

func NewCache(capacity int) *Cache {
	return &Cache{
		inner:          lru_cache.CreateCache[cachedEntry](capacity),
		negativeTTLCap: DefaultNegativeTTLCap,
		now:            time.Now,
	}
}

// NewCacheWithSWR builds a cache that serves entries past their TTL for up
// to staleGrace, returning them with RR.Ttl clamped to staleTTL. Pass
// staleGrace=0 to disable SWR (equivalent to NewCache).
func NewCacheWithSWR(capacity int, staleGrace, staleTTL time.Duration) *Cache {
	return &Cache{
		inner:          lru_cache.CreateCache[cachedEntry](capacity),
		negativeTTLCap: DefaultNegativeTTLCap,
		staleGrace:     staleGrace,
		staleTTL:       staleTTL,
		now:            time.Now,
	}
}

type AddResult struct {
	Cached  bool
	Evicted bool
	Size    int
}

type GetResult struct {
	Msg *dns.Msg
	// Hit means the entry is still within its authoritative TTL.
	Hit bool
	// Stale means the entry's TTL has elapsed but it is still inside the
	// SWR grace window; Msg is set with RR.Ttl clamped to StaleTTL so the
	// client comes back soon. Mutually exclusive with Hit and Expired.
	Stale bool
	// Expired means the entry exists but is past staleUntil — caller must
	// refresh from upstream. Mutually exclusive with Hit and Stale.
	Expired bool
}

// State is the high-level state of a cache lookup, exposed to consumers so
// they can branch on Fresh/Stale/Expired/Miss without unpacking the boolean
// flags of GetResult. Values are mutually exclusive.
type State int

const (
	StateMiss State = iota
	StateFresh
	StateStale
	StateExpired
)

// Lookup is the consumer-facing flavour of a cache read: a message and the
// state under which it was returned. Msg is nil for Miss and Expired.
type Lookup struct {
	Msg   *dns.Msg
	State State
}

// Add stores a deep copy of msg if it is cacheable. Returns Cached=false
// for responses we deliberately refuse to cache (SERVFAIL, missing TTL,
// negative response without SOA, …) so the metric layer can keep its
// counters honest.
//
// staleUntil = expiresAt + staleGrace for positive answers; for negative
// answers it equals expiresAt so they are never served stale.
func (c *Cache) Add(key string, msg *dns.Msg) AddResult {
	ttl, ok := computeCacheTTL(msg, c.negativeTTLCap)
	if !ok {
		return AddResult{}
	}
	now := c.now()
	expiresAt := now.Add(ttl)
	staleUntil := expiresAt
	if c.staleGrace > 0 && isPositiveAnswer(msg) {
		staleUntil = expiresAt.Add(c.staleGrace)
	}
	res := c.inner.Add(key, cachedEntry{
		msg:        msg.Copy(),
		cachedAt:   now,
		expiresAt:  expiresAt,
		staleUntil: staleUntil,
	})
	return AddResult{Cached: true, Evicted: res.Evicted, Size: res.Size}
}

// isPositiveAnswer reports whether the cached response is a real positive
// answer (NOERROR with at least one RR in Answer). Negative responses
// (NXDOMAIN, NODATA) must not be served stale — see the cachedEntry comment.
func isPositiveAnswer(msg *dns.Msg) bool {
	return msg.Rcode == dns.RcodeSuccess && len(msg.Answer) > 0
}

// Get returns a fresh, fully-owned copy of the cached message with every
// RR.Ttl decremented by the time it has spent in the cache. The result
// state is one of:
//   - Hit: now < expiresAt (authoritative TTL still valid).
//   - Stale: expiresAt <= now < staleUntil; Msg carries the cached payload
//     but with RR.Ttl clamped to staleTTL so the client comes back quickly
//     enough for an async refresh to land (RFC 8767).
//   - Expired: now >= staleUntil; entry exists but caller must refresh.
//
// An expired entry is left in place: the caller will Add a fresh upstream
// answer, which the LRU updates in-place under the same key, so the slot
// is not lost. Deleting here would race with a concurrent Add for the same
// key — a parallel goroutine could refresh the entry between our Get and
// Delete, and we'd then evict the freshly cached value.
func (c *Cache) Get(key string) GetResult {
	entry, ok := c.inner.Get(key)
	if !ok {
		return GetResult{}
	}
	now := c.now()
	if now.Before(entry.expiresAt) {
		msg := entry.msg.Copy()
		decrementTTL(msg, now.Sub(entry.cachedAt))
		return GetResult{Msg: msg, Hit: true}
	}
	if now.Before(entry.staleUntil) {
		msg := entry.msg.Copy()
		clampTTL(msg, c.staleTTL)
		return GetResult{Msg: msg, Stale: true}
	}
	return GetResult{Expired: true}
}

// Lookup wraps Get with an explicit State enum so consumers can switch on
// Fresh/Stale/Expired/Miss without inspecting three booleans.
func (c *Cache) Lookup(key string) Lookup {
	r := c.Get(key)
	switch {
	case r.Hit:
		return Lookup{Msg: r.Msg, State: StateFresh}
	case r.Stale:
		return Lookup{Msg: r.Msg, State: StateStale}
	case r.Expired:
		return Lookup{State: StateExpired}
	default:
		return Lookup{State: StateMiss}
	}
}

// Clear evicts every cached entry and returns how many were removed, so a
// manual-flush caller (e.g. the admin UI) can show "cleared N entries". It
// is safe to call concurrently with Get/Add — the LRU serialises mutations
// under its own mutex.
func (c *Cache) Clear() int {
	return c.inner.Clear()
}

// Len reports the current entry count. Useful for exposing the post-flush
// size and for assertions in tests.
func (c *Cache) Len() int {
	return c.inner.Len()
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

// clampTTL forces every RR.Ttl to min(currentTtl, capSeconds), with a floor
// of 1 (same reasoning as decrementTTL). Used for Stale responses where we
// want the client to come back quickly — RFC 8767 §6.
func clampTTL(msg *dns.Msg, cap time.Duration) {
	capSec := uint32(cap / time.Second)
	if capSec == 0 {
		capSec = 1
	}
	for _, section := range [][]dns.RR{msg.Answer, msg.Ns, msg.Extra} {
		for _, rr := range section {
			hdr := rr.Header()
			if hdr.Rrtype == dns.TypeOPT {
				continue
			}
			if hdr.Ttl == 0 || hdr.Ttl > capSec {
				hdr.Ttl = capSec
			}
		}
	}
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
