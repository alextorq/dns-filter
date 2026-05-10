package arpwatcher

import (
	"context"
	"errors"
	"time"

	"github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/discovery"
	"github.com/alextorq/dns-filter/clients/store"
)

// Logger is the minimum surface arpwatcher needs from the application
// logger. The package doesn't depend on the concrete logger type so tests
// can inject a no-op or capture impl without pulling the chan-based logger
// in.
type Logger interface {
	Info(args ...any)
	Warn(args ...any)
	Error(err error)
	Debug(args ...any)
}

// DefaultInterval is the cadence at which the watcher re-reads the kernel
// ARP table. Short enough that a DHCP renewal (typically minutes) is reflected
// quickly; long enough that the loop barely registers in CPU profile of a
// healthy host.
const DefaultInterval = 30 * time.Second

// Run blocks running the watcher loop until ctx is cancelled. On non-Linux
// platforms the first read returns ErrUnsupported and the loop exits without
// further attempts — there's no point retrying on a host that fundamentally
// doesn't expose /proc/net/arp.
//
// Pass DefaultInterval (or a shorter value in tests) for the refresh cadence.
func Run(ctx context.Context, log Logger, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultInterval
	}

	// Run an immediate first pass so the cache isn't empty for the first
	// `interval` seconds after startup, then settle into the timer cadence.
	if !tick(ctx, log) {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !tick(ctx, log) {
				return
			}
		}
	}
}

// tick reads the ARP table once and folds the result into the cache. It
// returns false to signal "give up the loop" — currently only when the
// platform doesn't support ARP reading.
func tick(ctx context.Context, log Logger) bool {
	entries, err := discovery.ReadARPTable()
	if err != nil {
		if errors.Is(err, discovery.ErrUnsupported) {
			log.Warn("arpwatcher: platform unsupported, stopping watcher loop")
			return false
		}
		log.Error(err)
		return true
	}

	res := Get().Update(entries)
	if res.NewPairs == 0 && res.ChangedIPs == 0 && res.ChangedMACs == 0 {
		return true
	}

	log.Debug("arpwatcher: cache update",
		"new", res.NewPairs,
		"changed_ip", res.ChangedIPs,
		"changed_mac", res.ChangedMACs,
		"known", res.TotalKnown,
	)

	if backfilled := backfillClients(ctx); backfilled > 0 {
		// Rebuild the in-memory exclusion snapshot so newly-attached MACs
		// participate in the hot-path lookup. The store rebuild is cheap
		// (a single SELECT over a small table); doing it after a batch
		// rather than per-row keeps the disruption minimal.
		if err := store.Get().UpdateFromDB(); err != nil {
			log.Error(err)
		}
		log.Info("arpwatcher: backfilled MACs for", backfilled, "client(s)")
	}
	return true
}

// backfillClients fills in the MAC field for any Client with a known IP but
// empty MAC. Existing MACs are never overwritten — if the user manually set
// a MAC and the kernel later reports a different one for that IP, we keep
// the user's value rather than guess which side is right.
//
// Returns the number of rows updated.
func backfillClients(_ context.Context) int {
	pairs := Get().Pairs()
	if len(pairs) == 0 {
		return 0
	}
	clients, err := db.GetAllClients()
	if err != nil {
		return 0
	}
	updated := 0
	for _, c := range clients {
		if c.IP == "" || c.MAC != "" {
			continue
		}
		mac, ok := pairs[c.IP]
		if !ok {
			continue
		}
		if err := db.UpdateClientFields(c.ID, map[string]any{"mac": mac}); err != nil {
			continue
		}
		updated++
	}
	return updated
}
