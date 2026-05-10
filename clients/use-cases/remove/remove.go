package remove

import (
	"github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/store"
)

// Remove deletes the client and drops its identifiers from the in-memory
// exclusion store. The store cleanup runs even if the row was already
// Filtered=true (i.e., never excluded) — RemoveClient is idempotent.
func Remove(id uint) error {
	c, err := db.GetClientByID(id)
	if err != nil {
		return err
	}
	if err := db.DeleteClient(id); err != nil {
		return err
	}
	store.Get().RemoveClient(c)
	return nil
}
