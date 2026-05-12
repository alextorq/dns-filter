package dns

import (
	"github.com/alextorq/dns-filter/metric"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/singleflight"
)

// singleflightCoalesced counts DNS queries that returned a result shared with
// other concurrent callers — incremented per caller (including the owner of
// the group), not per saved upstream call. For a coalesced group of N callers
// the counter grows by N, while the number of upstream calls avoided is N-1.
// Spikes here mean a popular domain just fell out of cache and many clients
// piled on at once — exactly the thundering-herd this coordinator absorbs.
var singleflightCoalesced = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "dns_singleflight_coalesced_total",
	Help: "DNS queries that received a singleflight-shared upstream result (counts every caller in a coalesced group, including the owner; saved upstream calls = value minus number of groups)",
})

func init() {
	metric.Registry.MustRegister(singleflightCoalesced)
}

// upstreamCoordinator collapses concurrent identical queries into a single
// in-flight upstream call. The key is the same (name+qtype) we use for the
// DNS cache, so coalescing aligns with the cache's notion of identity.
type upstreamCoordinator struct {
	group singleflight.Group
}

// Do runs fn under the singleflight key. If another caller is already running
// the same key, this caller waits and receives the same result. Because
// singleflight returns the same *dns.Msg pointer to every shared caller and
// the DNS hot path mutates msg.Id per caller, we deep-copy the result on
// shared returns to keep callers from racing on header fields.
func (c *upstreamCoordinator) Do(key string, fn func() (*dns.Msg, error)) (*dns.Msg, error) {
	v, err, shared := c.group.Do(key, func() (any, error) {
		return fn()
	})
	if shared {
		singleflightCoalesced.Inc()
	}
	if err != nil {
		return nil, err
	}
	msg := v.(*dns.Msg)
	if shared {
		msg = msg.Copy()
	}
	return msg, nil
}
