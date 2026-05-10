// Package arpwatcher keeps a live IP↔MAC map of the local LAN by periodically
// re-reading the kernel's ARP cache. The DNS hot path consults the resulting
// Cache to translate a query's source IP into the MAC of the device that
// sent it; client exclusion rules then survive DHCP IP rotation, because the
// MAC is the stable identifier and IP is just the latest courier address.
//
// The watcher is best-effort: errors don't propagate, the cache simply stays
// stale until the next refresh succeeds. On non-Linux platforms /proc/net/arp
// doesn't exist, so the watcher detects ErrUnsupported on first read and
// stops looping rather than spamming logs every tick.
package arpwatcher

import (
	"maps"
	"sync"
	"time"

	"github.com/alextorq/dns-filter/clients/discovery"
)

// Cache is the bidirectional IP↔MAC map. The DNS hot path reads it via
// Cache.MAC(ip); the watcher writes via Update. Reads vastly outnumber
// writes (a /24 LAN refreshed every 30s = ~250 entries replaced ~2/min,
// vs. potentially thousands of DNS queries per minute), so RWMutex is the
// right primitive.
type Cache struct {
	mu          sync.RWMutex
	ipToMAC     map[string]string
	macToIP     map[string]string
	lastUpdated time.Time
	knownCount  int
}

var (
	instance *Cache
	once     sync.Once
)

// Get returns the singleton Cache. Both the watcher and the IPIdentifier
// share one instance so the hot path sees what the watcher has learned.
func Get() *Cache {
	once.Do(func() {
		instance = &Cache{
			ipToMAC: map[string]string{},
			macToIP: map[string]string{},
		}
	})
	return instance
}

// MAC returns the MAC last associated with the given IP, or "" if unknown.
// This is the hot-path query — keep it fast and lock-only-on-read.
func (c *Cache) MAC(ip string) (string, bool) {
	if ip == "" {
		return "", false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	mac, ok := c.ipToMAC[ip]
	return mac, ok
}

// IP returns the IP last associated with the given MAC, or "" if unknown.
// Used by the backfill path: when the watcher sees a (IP, MAC) pair, the
// MAC-only client (if any) gets its IP updated to the latest courier.
func (c *Cache) IP(mac string) (string, bool) {
	if mac == "" {
		return "", false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	ip, ok := c.macToIP[mac]
	return ip, ok
}

// UpdateResult summarizes what changed during one Update call. The watcher
// uses NewPairs to decide whether to trigger a backfill pass on the DB.
type UpdateResult struct {
	NewPairs       int // (IP, MAC) pairs the cache hadn't seen
	ChangedIPs     int // existing MACs whose IP moved (DHCP rotation)
	ChangedMACs    int // existing IPs whose MAC changed (different device on same IP)
	TotalKnown     int
	UpdatedAtTime  time.Time
}

// Update applies a fresh snapshot of ARP table entries to the cache. It is
// not idempotent in the strict sense — the cache's lastUpdated timestamp
// always advances — but feeding the same entries twice produces zero
// new/changed pairs. The watcher passes the union of /proc/net/arp results
// each tick rather than diffs, so this function does the diffing itself.
func (c *Cache) Update(entries []discovery.ARPEntry) UpdateResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	res := UpdateResult{}
	for _, e := range entries {
		if e.IP == nil || e.MAC == nil {
			continue
		}
		ip := e.IP.String()
		mac := e.MAC.String()
		if ip == "" || mac == "" {
			continue
		}
		oldMAC, hadIP := c.ipToMAC[ip]
		oldIP, hadMAC := c.macToIP[mac]
		switch {
		case !hadIP && !hadMAC:
			res.NewPairs++
		case hadIP && oldMAC != mac:
			// Same IP, different MAC: a new device took over the address.
			res.ChangedMACs++
			delete(c.macToIP, oldMAC)
		case hadMAC && oldIP != ip:
			// Same MAC, different IP: DHCP rotation, the device moved.
			res.ChangedIPs++
			delete(c.ipToMAC, oldIP)
		}
		c.ipToMAC[ip] = mac
		c.macToIP[mac] = ip
	}
	c.lastUpdated = time.Now()
	c.knownCount = len(c.ipToMAC)
	res.TotalKnown = c.knownCount
	res.UpdatedAtTime = c.lastUpdated
	return res
}

// Stats returns a point-in-time snapshot for diagnostics. The watcher logs
// these after each refresh; the frontend (future) could display them.
type Stats struct {
	Known       int
	LastUpdated time.Time
}

func (c *Cache) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Stats{Known: c.knownCount, LastUpdated: c.lastUpdated}
}

// Pairs returns a snapshot of every IP→MAC entry. Used by the backfill pass
// to scan for IP-only clients that can now have a MAC populated.
func (c *Cache) Pairs() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]string, len(c.ipToMAC))
	maps.Copy(out, c.ipToMAC)
	return out
}
