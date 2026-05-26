// Package hostnames runs a background sweep that learns friendly device names
// for the LAN and persists them as MAC→hostname rows (clients/hostnames/db).
//
// The sweep is mDNS-only: it browses the network for self-announcing devices
// (Apple gear, printers, TVs, Chromecast, NAS) and resolves each announced IP
// to a MAC through the live arpwatcher cache — it deliberately does NOT run the
// heavier active-ARP broadcast that on-demand discovery does, because the
// arpwatcher already maintains IP↔MAC passively. A host whose MAC is not known
// at sweep time is skipped rather than stored under its IP; the next sweep
// picks it up once the arpwatcher has learned the pair. Coverage is therefore
// partial by nature: devices that announce nothing over mDNS (many Android
// phones, some IoT) never get a name here and fall back to vendor/IP in the UI.
//
// Like the other background workers (arpwatcher, source sync, traffic prune)
// this is started only in LAN mode and only matters there — there is no LAN to
// browse behind a public DoH endpoint.
package hostnames

import (
	"context"
	"time"

	"github.com/alextorq/dns-filter/clients/discovery"
)

// Default cadence and retention. Browsing is cheap but not free, and device
// names change rarely, so a 10-minute sweep is ample. A device unseen for the
// TTL is pruned; because rows are MAC-keyed (stable identifiers) the TTL is
// just garbage collection for departed devices, not a staleness guard.
const (
	DefaultInterval = 10 * time.Minute
	DefaultTTL      = 30 * 24 * time.Hour
)

// MACLookup resolves an IP to the MAC currently bound to it on the LAN.
// arpwatcher.Cache satisfies it. Kept as a narrow interface so the collector
// stays testable and does not import the watcher's concrete type.
type MACLookup interface {
	MAC(ip string) (string, bool)
}

// Store persists the learned pairs. clients/hostnames/db.Repo satisfies it.
type Store interface {
	Upsert(mac, hostname string) error
	PruneOlderThan(window time.Duration) error
}

// Browser runs one multicast browse and returns the IP→hostname pairs.
// discovery.BrowseMDNS satisfies it.
type Browser func(ctx context.Context) ([]discovery.MDNSHost, error)

// Logger is the narrow logging port the collector needs.
type Logger interface {
	Info(args ...any)
	Error(err error)
}

// Collector wires the sweep dependencies. Construct one at the composition root
// and call Run in a goroutine. Interval/TTL default to the package constants
// when left zero.
type Collector struct {
	Browse   Browser
	MACs     MACLookup
	Store    Store
	Log      Logger
	Interval time.Duration
	TTL      time.Duration
}

// Run sweeps immediately, then on every Interval tick until ctx is cancelled.
// The immediate sweep means names start appearing without waiting a full
// interval; at boot the arpwatcher may not have learned MACs yet, so that first
// sweep may resolve nothing — which is fine, since unresolved hosts are skipped
// (never mis-keyed) and the next tick catches them.
func (c *Collector) Run(ctx context.Context) {
	interval := c.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	c.Log.Info("hostname collector started (mDNS sweep every", interval, ")")

	c.sweep(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sweep(ctx)
		}
	}
}

// sweep browses once, upserts a name for every host whose IP resolves to a
// known MAC, then prunes departed devices. A partial mDNS error (some service
// browses failed) is logged but does not abort the sweep — the hosts that did
// answer are still recorded.
func (c *Collector) sweep(ctx context.Context) {
	hosts, err := c.Browse(ctx)
	if err != nil {
		c.Log.Error(err)
		// fall through: hosts may still hold partial results
	}

	recorded := 0
	for _, h := range hosts {
		if h.Hostname == "" || c.MACs == nil {
			continue
		}
		mac, ok := c.MACs.MAC(h.IP)
		if !ok || mac == "" {
			continue // MAC unknown — skip rather than key by a rotating IP
		}
		if err := c.Store.Upsert(mac, h.Hostname); err != nil {
			c.Log.Error(err)
			continue
		}
		recorded++
	}

	ttl := c.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if err := c.Store.PruneOlderThan(ttl); err != nil {
		c.Log.Error(err)
	}
	if recorded > 0 {
		c.Log.Info("hostname collector recorded", recorded, "device name(s)")
	}
}
