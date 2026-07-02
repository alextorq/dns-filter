// Package change_filter toggles whether DNS filtering is applied to a client.
//
// Filtered=true is the normal state — queries from this client go through the
// blocklist as usual. Filtered=false excludes the client from the filter; the
// hot path skips the bloom/blocklist check entirely.
//
// The use case touches two pieces of state — the DB row and the in-memory
// exclusion store — and they must agree at the end. clients.Module serializes
// this operation with other exclusion mutations and full snapshot rebuilds.
package change_filter

import "github.com/alextorq/dns-filter/clients/db"

type Repo interface {
	GetByID(id uint) (*db.Client, error)
	UpdateFields(id uint, fields map[string]any) error
}

type ExclusionStore interface {
	AddClient(*db.Client)
	RemoveClient(*db.Client)
}

func ChangeFilter(repo Repo, exclusions ExclusionStore, id uint, filtered bool) (*db.Client, error) {
	c, err := repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if err := repo.UpdateFields(id, map[string]any{"filtered": filtered}); err != nil {
		return nil, err
	}
	c.Filtered = filtered

	if filtered {
		exclusions.RemoveClient(c)
	} else {
		exclusions.AddClient(c)
	}
	return c, nil
}
