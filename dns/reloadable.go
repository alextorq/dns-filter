package dns

import (
	"context"
	"sync"
	"sync/atomic"

	dnsLib "github.com/miekg/dns"
)

// ReloadableResolver is an UpstreamResolver whose underlying DoH resolver can
// be swapped at runtime without restarting the DNS server.
//
// The hot path reads the current resolver through an atomic pointer
// (lock-free); a settings change rebuilds a fresh *DoHResolver from the stored
// endpoint + bootstrap IPs and atomically swaps it in. The same instance is
// shared by the DNS server and the SWR refresh worker, so a single swap
// repoints both.
//
// Endpoint and bootstrap IPs are owned here (under mu) rather than read back
// from the settings module, so the two settings — doh_upstream and
// doh_bootstrap_ips — can be applied independently without either having to
// know the other's current value (and without re-entering the settings
// module's lock).
type ReloadableResolver struct {
	mu       sync.Mutex
	endpoint string
	bootIPs  []string
	inner    atomic.Pointer[DoHResolver]
}

// NewReloadableResolver builds the initial resolver from endpoint + bootstrap
// IPs. Safe to use immediately; later swaps go through SetEndpoint /
// SetBootstrapIPs.
func NewReloadableResolver(endpoint string, bootstrapIPs ...string) *ReloadableResolver {
	r := &ReloadableResolver{
		endpoint: endpoint,
		bootIPs:  append([]string(nil), bootstrapIPs...),
	}
	r.rebuildLocked()
	return r
}

// rebuildLocked constructs a fresh *DoHResolver from the current endpoint +
// bootstrap IPs and stores it atomically. Caller must hold mu (the constructor
// runs single-threaded and is exempt).
func (r *ReloadableResolver) rebuildLocked() {
	r.inner.Store(NewDoHResolver(r.endpoint, r.bootIPs...))
}

// SetEndpoint swaps the upstream endpoint, rebuilding the resolver from the
// new endpoint and the current bootstrap IPs.
func (r *ReloadableResolver) SetEndpoint(endpoint string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endpoint = endpoint
	r.rebuildLocked()
}

// SetBootstrapIPs swaps the bootstrap IPs, rebuilding the resolver from the
// current endpoint and the new IPs.
func (r *ReloadableResolver) SetBootstrapIPs(ips []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bootIPs = append([]string(nil), ips...)
	r.rebuildLocked()
}

// Exchange satisfies UpstreamResolver by delegating to the resolver currently
// in effect. The atomic load means an in-flight swap never tears the pointer.
func (r *ReloadableResolver) Exchange(ctx context.Context, msg *dnsLib.Msg) (*dnsLib.Msg, error) {
	return r.inner.Load().Exchange(ctx, msg)
}
