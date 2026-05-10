// Package identifier resolves an inbound DNS request to a stable client
// handle. The hot path uses the result as a lookup key in the in-memory
// exclusion store — separating "how do we recognize this client" from "is
// this client excluded" lets us add new transports (DoH-over-HTTPS in the
// future "public" mode) without touching the hot path.
package identifier

import "net"

// Kinds enumerates the identifier flavors supported by the store.
const (
	KindIP    = "ip"
	KindMAC   = "mac"
	KindToken = "token"
)

// Lookup is the canonical key the store understands. Two lookups with the
// same Kind+Value refer to the same client; the kinds are distinct namespaces
// (an IP "10.0.0.1" never collides with a token "10.0.0.1").
type Lookup struct {
	Kind  string
	Value string
}

// Request is everything an Identifier may need from a transport. The fields
// correspond to the two transports we care about today and in the near future:
//   - LAN (UDP/TCP): RemoteAddr is host:port of the DNS client.
//   - Public (DoH):  URLPath carries the per-client token.
//
// Adding a third transport later means adding a field here, not changing the
// signature.
type Request struct {
	RemoteAddr string
	URLPath    string
}

// Identifier converts a transport-specific Request into a Lookup. The bool
// return signals "could not identify" — in that case the hot path falls back
// to the global filter policy without consulting the store.
type Identifier interface {
	Identify(Request) (Lookup, bool)
}

// IPIdentifier resolves the request's remote address to an IP-keyed Lookup.
// In a later PR an ARP cache will be plugged in here so the store key becomes
// a MAC when one is known — the hot path stays unchanged.
type IPIdentifier struct{}

func (IPIdentifier) Identify(r Request) (Lookup, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return Lookup{}, false
	}
	if host == "" {
		return Lookup{}, false
	}
	return Lookup{Kind: KindIP, Value: host}, true
}
