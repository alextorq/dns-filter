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

// MACResolver looks up the MAC currently bound to an IP in the local LAN.
// The arpwatcher package implements this; the interface lives here so the
// hot path doesn't have to import arpwatcher directly (and so tests can
// inject a stub without spinning up the singleton).
type MACResolver interface {
	MAC(ip string) (string, bool)
}

// IPIdentifier resolves the request's remote address to a Lookup.
//
// When a Resolver is provided and knows a MAC for the source IP, the result
// is a KindMAC lookup — that's what makes filter rules survive DHCP IP
// rotation, since MAC is the stable identifier and IP is just whatever the
// device was last assigned. Without a resolver (or when the MAC is unknown,
// e.g. before the watcher's first refresh), the result falls back to the IP.
type IPIdentifier struct {
	Resolver MACResolver
}

func (i IPIdentifier) Identify(r Request) (Lookup, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return Lookup{}, false
	}
	if host == "" {
		return Lookup{}, false
	}
	if i.Resolver != nil {
		if mac, ok := i.Resolver.MAC(host); ok && mac != "" {
			return Lookup{Kind: KindMAC, Value: mac}, true
		}
	}
	return Lookup{Kind: KindIP, Value: host}, true
}
