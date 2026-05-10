//go:build !linux

package discovery

import (
	"context"
	"errors"
)

// runARPDiscovery is a stub on non-Linux platforms. /proc/net/arp doesn't
// exist on macOS/Windows, and the active-scan path uses AF_PACKET which is
// Linux-specific. Local development on a Mac builds and runs the rest of
// the binary fine; only LAN discovery degrades to "no entries" with a clear
// error so the operator knows why the Network scan tab is empty.
func runARPDiscovery(_ context.Context, _ *LocalSubnet) scanResult {
	return scanResult{
		Errors: []error{errors.New("ARP discovery requires Linux (build platform mismatch)")},
	}
}
