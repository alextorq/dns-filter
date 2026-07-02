package remove

import (
	"github.com/alextorq/dns-filter/clients/db"
)

type Repo interface {
	GetByID(id uint) (*db.Client, error)
	Delete(id uint) error
}

type ExclusionStore interface {
	RemoveClient(*db.Client)
}

// Remove deletes the client and drops its identifiers from the in-memory
// exclusion store. The store cleanup runs even if the row was already
// Filtered=true (i.e., never excluded) — RemoveClient is idempotent.
func Remove(repo Repo, exclusions ExclusionStore, id uint) error {
	c, err := repo.GetByID(id)
	if err != nil {
		return err
	}
	if err := repo.Delete(id); err != nil {
		return err
	}
	exclusions.RemoveClient(c)
	return nil
}
