package arpwatcher

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/discovery"
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

type ClientRepo interface {
	GetAll() ([]db.Client, error)
	UpdateFields(id uint, fields map[string]any) error
}

// Watcher owns every dependency used by the periodic ARP refresh. The cache is
// shared with the DNS hot path; SyncExclusions rebuilds the client exclusion
// snapshot after MAC backfill changes the canonical lookup key.
type Watcher struct {
	Cache          *Cache
	Repo           ClientRepo
	SyncExclusions func() error
	ReadARP        func() ([]discovery.ARPEntry, error)
	FilterARP      func([]discovery.ARPEntry) []discovery.ARPEntry
}

func NewWatcher(cache *Cache, repo ClientRepo, syncExclusions func() error) *Watcher {
	return &Watcher{
		Cache:          cache,
		Repo:           repo,
		SyncExclusions: syncExclusions,
		ReadARP:        discovery.ReadARPTable,
		FilterARP:      discovery.FilterDockerARP,
	}
}

// Run blocks running the watcher loop until ctx is cancelled. On non-Linux
// platforms the first read returns ErrUnsupported and the loop exits without
// further attempts — there's no point retrying on a host that fundamentally
// doesn't expose /proc/net/arp.
//
// Pass DefaultInterval (or a shorter value in tests) for the refresh cadence.
func (w *Watcher) Run(ctx context.Context, log Logger, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultInterval
	}

	// Run an immediate first pass so the cache isn't empty for the first
	// `interval` seconds after startup, then settle into the timer cadence.
	if !w.tick(log) {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !w.tick(log) {
				return
			}
		}
	}
}

// tick reads the ARP table once and folds the result into the cache. It
// returns false to signal "give up the loop" — currently only when the
// platform doesn't support ARP reading.
func (w *Watcher) tick(log Logger) bool {
	entries, err := w.ReadARP()
	if err != nil {
		if errors.Is(err, discovery.ErrUnsupported) {
			log.Warn("arpwatcher: platform unsupported, stopping watcher loop")
			return false
		}
		log.Error(err)
		return true
	}

	// The watcher's IP↔MAC cache only backfills real client MACs, so Docker
	// neighbours are pure noise here — drop them.
	entries = w.FilterARP(entries)

	res := w.Cache.Update(entries)
	if res.NewPairs == 0 && res.ChangedIPs == 0 && res.ChangedMACs == 0 {
		return true
	}

	log.Debug("arpwatcher: cache update",
		"new", res.NewPairs,
		"changed_ip", res.ChangedIPs,
		"changed_mac", res.ChangedMACs,
		"known", res.TotalKnown,
	)

	if backfilled := w.backfillClients(log); backfilled > 0 {
		// Rebuild the in-memory exclusion snapshot so newly-attached MACs
		// participate in the hot-path lookup. The store rebuild is cheap
		// (a single SELECT over a small table); doing it after a batch
		// rather than per-row keeps the disruption minimal.
		if err := w.SyncExclusions(); err != nil {
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
// DB read/write failures are logged but never abort the loop: a single
// row's UPDATE failing under contention shouldn't stop the rest of the
// batch, and the operator needs the log line to know why the MAC column
// stayed empty.
//
// Returns the number of rows updated.
func (w *Watcher) backfillClients(log Logger) int {
	pairs := w.Cache.Pairs()
	if len(pairs) == 0 {
		return 0
	}
	clients, err := w.Repo.GetAll()
	if err != nil {
		log.Warn("arpwatcher: backfill list clients failed:", err)
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
		if err := w.Repo.UpdateFields(c.ID, map[string]any{"mac": mac}); err != nil {
			log.Warn(fmt.Sprintf("arpwatcher: backfill update client %d (%s): %v", c.ID, c.IP, err))
			continue
		}
		updated++
	}
	return updated
}
