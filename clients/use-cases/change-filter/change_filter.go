// Package change_filter toggles whether DNS filtering is applied to a client.
//
// Filtered=true is the normal state — queries from this client go through the
// blocklist as usual. Filtered=false excludes the client from the filter; the
// hot path skips the bloom/blocklist check entirely.
//
// The use case touches two pieces of state — the DB row and the in-memory
// exclusion store — and they must agree at the end. A package-level mutex
// serializes calls so that two concurrent toggles on the same id can't have
// their store mutations land in the wrong order, leaving DB and memory
// disagreeing until the next UpdateFromDB pass.
package change_filter

import (
	"sync"

	"github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/store"
)

var mu sync.Mutex

func ChangeFilter(id uint, filtered bool) (*db.Client, error) {
	mu.Lock()
	defer mu.Unlock()

	c, err := db.GetClientByID(id)
	if err != nil {
		return nil, err
	}
	if err := db.UpdateClientFields(id, map[string]any{"filtered": filtered}); err != nil {
		return nil, err
	}
	c.Filtered = filtered

	s := store.Get()
	if filtered {
		s.RemoveClient(c)
	} else {
		s.AddClient(c)
	}
	return c, nil
}
