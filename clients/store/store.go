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

// UpdateFromDB rebuilds the snapshot from the database. Called once at boot
// and any time a bulk operation makes incremental updates impractical.
func (s *Store) UpdateFromDB() error {
	rows, err := db.GetExcludedClients()
	if err != nil {
		return err
	}
	next := make(map[string]struct{}, len(rows)*2)
	for _, c := range rows {
		// A client may have multiple identifiers populated (e.g. both IP and
		// MAC after the PR3 ARP watcher fills MAC for an existing IP-only
		// row). All of them resolve to the same exclusion verdict, so we
		// register each non-empty one.
		if c.IP != "" {
			next[identifier.KindIP+":"+c.IP] = struct{}{}
		}
		if c.MAC != "" {
			next[identifier.KindMAC+":"+c.MAC] = struct{}{}
		}
		if c.Token != "" {
			next[identifier.KindToken+":"+c.Token] = struct{}{}
		}
	}
	s.mu.Lock()
	s.excluded = next
	s.mu.Unlock()
	return nil
}

// AddClient registers each non-empty identifier of c. Called by the create
// use case when a client is added with Filtered=false.
func (s *Store) AddClient(c *db.Client) {
	if c == nil {
		return
	}
	if c.IP != "" {
		s.Add(identifier.Lookup{Kind: identifier.KindIP, Value: c.IP})
	}
	if c.MAC != "" {
		s.Add(identifier.Lookup{Kind: identifier.KindMAC, Value: c.MAC})
	}
	if c.Token != "" {
		s.Add(identifier.Lookup{Kind: identifier.KindToken, Value: c.Token})
	}
}

// RemoveClient drops every identifier of c from the exclusion set.
func (s *Store) RemoveClient(c *db.Client) {
	if c == nil {
		return
	}
	if c.IP != "" {
		s.Remove(identifier.Lookup{Kind: identifier.KindIP, Value: c.IP})
	}
	if c.MAC != "" {
		s.Remove(identifier.Lookup{Kind: identifier.KindMAC, Value: c.MAC})
	}
	if c.Token != "" {
		s.Remove(identifier.Lookup{Kind: identifier.KindToken, Value: c.Token})
	}
}
