// Package clients is the public façade other packages call into. It exposes
// the few operations main.go and the HTTP layer need (Sync at boot, the CRUD
// verbs) without forcing them to know about the internal split between
// identifier, store, db, and use-cases packages.
package clients

import (
	"github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/store"
	change_filter "github.com/alextorq/dns-filter/clients/use-cases/change-filter"
	"github.com/alextorq/dns-filter/clients/use-cases/create"
	"github.com/alextorq/dns-filter/clients/use-cases/remove"
	"github.com/alextorq/dns-filter/clients/use-cases/update"
)

// Sync rebuilds the in-memory exclusion snapshot from the database. Called at
// startup before the DNS server begins accepting traffic, and any time bulk
// changes make incremental updates impractical.
func Sync() error {
	return store.Get().UpdateFromDB()
}

// Create registers a new client. See create.Input for the field semantics.
func Create(in create.Input) (*db.Client, error) {
	return create.Create(in)
}

// Update mutates user-editable metadata. Filtered is intentionally not exposed
// here — use ChangeFilter, which keeps the in-memory store consistent.
func Update(in update.Input) (*db.Client, error) {
	return update.Update(in)
}

// ChangeFilter toggles the DNS filter for a single client.
func ChangeFilter(id uint, filtered bool) (*db.Client, error) {
	return change_filter.ChangeFilter(id, filtered)
}

// Remove deletes the client and drops it from the exclusion snapshot.
func Remove(id uint) error {
	return remove.Remove(id)
}
