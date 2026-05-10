package discovery

import "net"

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
