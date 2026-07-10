package dns

import (
	"github.com/miekg/dns"
	"golang.org/x/sync/singleflight"
)

// upstreamCoordinator collapses concurrent identical queries into a single
// in-flight upstream call. The key is the same (name+qtype) we use for the
// DNS cache, so coalescing aligns with the cache's notion of identity.
type upstreamCoordinator struct {
	group  singleflight.Group
	metric Metric
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
	if shared && c.metric != nil {
		c.metric.IncSingleflightCoalesced()
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
