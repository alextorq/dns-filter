// Package store keeps an in-memory snapshot of the client exclusion list and
// answers the single question the DNS hot path asks: "is this client excluded
// from filtering?". It exists so the hot path never has to touch SQLite, and
// the snapshot is rebuilt from the DB at startup and on every CRUD mutation.
package store

import (
	"sync"

	"github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/identifier"
)

// Store is the singleton in-memory exclusion set. Keys are
// "<kind>:<value>" so that lookups by IP, MAC, or token live in the same map
// without colliding.
type Store struct {
	mu       sync.RWMutex
	excluded map[string]struct{}
}

var (
	instance *Store
	once     sync.Once
)

// Get returns the singleton Store. Multiple callers share one set so the DNS
// hot path and the HTTP CRUD handlers see the same state.
func Get() *Store {
	once.Do(func() {
		instance = &Store{excluded: make(map[string]struct{})}
	})
	return instance
}

func key(l identifier.Lookup) string {
	return l.Kind + ":" + l.Value
}

// IsExcluded is the hot-path query: O(1) lookup, no DB.
func (s *Store) IsExcluded(l identifier.Lookup) bool {
	if l.Kind == "" || l.Value == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.excluded[key(l)]
	return ok
}

// Add records the lookup as excluded. Used after a CRUD mutation so the hot
// path picks up the change without waiting for a full UpdateFromDB pass.
func (s *Store) Add(l identifier.Lookup) {
	if l.Kind == "" || l.Value == "" {
		return
	}
	s.mu.Lock()
	s.excluded[key(l)] = struct{}{}
	s.mu.Unlock()
}

// Remove drops the lookup from the exclusion set.
func (s *Store) Remove(l identifier.Lookup) {
	if l.Kind == "" || l.Value == "" {
		return
	}
	s.mu.Lock()
	delete(s.excluded, key(l))
	s.mu.Unlock()
}

// clientLookups returns the canonical exclusion lookups for a client.
//
// The MAC-trumps-IP rule is load-bearing: once the ARP watcher fills the
// MAC field, IP becomes informational, and we deliberately stop keying the
// store entry on IP. Otherwise a stale IP-based exclusion would silently
// match a NEW device that DHCP later assigned the old IP to — the device
// would bypass filtering even though the user's rule was meant for someone
// else. With MAC-only entries, the hot path resolves IP→MAC via the live
// ARP cache before consulting the store, so a new device with a different
// MAC at the same IP correctly misses.
//
// Token entries are independent of the LAN identifiers and always registered
// when present (public mode lives in its own namespace).
func clientLookups(c *db.Client) []identifier.Lookup {
	if c == nil {
		return nil
	}
	var out []identifier.Lookup
	if c.Token != "" {
		out = append(out, identifier.Lookup{Kind: identifier.KindToken, Value: c.Token})
	}
	switch {
	case c.MAC != "":
		out = append(out, identifier.Lookup{Kind: identifier.KindMAC, Value: c.MAC})
	case c.IP != "":
		out = append(out, identifier.Lookup{Kind: identifier.KindIP, Value: c.IP})
	}
	return out
}

// UpdateFromDB rebuilds the snapshot from the database. Called once at boot
// and any time a bulk operation makes incremental updates impractical.
func (s *Store) UpdateFromDB() error {
	rows, err := db.GetExcludedClients()
	if err != nil {
		return err
	}
	next := make(map[string]struct{}, len(rows)*2)
	for _, c := range rows {
		for _, l := range clientLookups(&c) {
			next[key(l)] = struct{}{}
		}
	}
	s.mu.Lock()
	s.excluded = next
	s.mu.Unlock()
	return nil
}

// AddClient registers the canonical exclusion lookups for c. Called by the
// create / change-filter use cases.
func (s *Store) AddClient(c *db.Client) {
	for _, l := range clientLookups(c) {
		s.Add(l)
	}
}

// RemoveClient drops the canonical exclusion lookups for c.
func (s *Store) RemoveClient(c *db.Client) {
	for _, l := range clientLookups(c) {
		s.Remove(l)
	}
}
