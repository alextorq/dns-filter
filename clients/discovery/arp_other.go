//go:build !linux

package discovery

import "context"

// ReadARPTable on non-Linux platforms is a stub. /proc/net/arp doesn't exist
// outside Linux; mDNS-only discovery still works in the on-demand path. The
// bool is filterDocker, ignored here since there is no ARP table to read.
func ReadARPTable(_ bool) ([]ARPEntry, error) {
	return nil, ErrUnsupported
}

// runARPDiscovery is a stub on non-Linux platforms. The active-scan path uses
// AF_PACKET which is Linux-specific, and the passive path needs /proc/net/arp.
// Local development on a Mac builds and runs the rest of the binary fine;
// only LAN discovery degrades to "no entries" with a clear error so the
// operator knows why the Network scan tab is empty.
func runARPDiscovery(_ context.Context, _ *LocalSubnet, _ DiscoverOptions) scanResult {
	return scanResult{Errors: []error{ErrUnsupported}}
}
