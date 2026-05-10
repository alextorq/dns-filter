package discovery

import (
	"errors"
	"net"
)

// ErrUnsupported signals that ARP-based discovery isn't available on the
// current platform. The arpwatcher uses errors.Is to detect this and exit
// its periodic loop early — there's no point hammering ReadARPTable every
// 30s when the platform fundamentally lacks /proc/net/arp.
//
// On Linux this error is never returned. The variable is declared here so
// callers can reference it without build tags.
var ErrUnsupported = errors.New("ARP discovery requires Linux (build platform mismatch)")

// ARPEntry is one (IP, MAC) pair learned from either the passive read of
// /proc/net/arp or an active broadcast scan.
type ARPEntry struct {
	IP     net.IP
	MAC    net.HardwareAddr
	Source string // "arp-table" | "active-scan"
}

// scanResult bundles the entries found and a list of partial errors. Active
// scan is best-effort: a permission error or a missing interface should not
// abandon the passive results we already collected.
type scanResult struct {
	Entries []ARPEntry
	Errors  []error
}
