package update_dns_record

import (
	"fmt"

	"github.com/alextorq/dns-filter/blocked-domain/db"
)

type UpdateBlockList struct {
	ID     uint `json:"id" binding:"required"`
	Active bool `json:"active"`
}

// Repo is the output port for this use-case. *blocked-domain/db.Repo satisfies
// it via structural typing.
type Repo interface {
	GetByID(id uint) (*db.BlockList, error)
	UpdateBlockList(*db.BlockList) error
}

type Logger interface {
	Info(args ...any)
	Error(err error)
}

type Deps struct {
	Repo Repo
	Log  Logger
}

func UpdateDnsRecord(d Deps, update UpdateBlockList) (*db.BlockList, error) {
	record, err := d.Repo.GetByID(update.ID)
	if err != nil {
		wrap := fmt.Errorf("error get record by id when change record: %w", err)
		d.Log.Error(wrap)
		return nil, wrap
	}

	record.Active = update.Active

	if err := d.Repo.UpdateBlockList(record); err != nil {
		wrap := fmt.Errorf("error update record when change record: %w", err)
		d.Log.Error(wrap)
		return nil, wrap
	}
	d.Log.Info("Record updated:", record)
	return record, nil
}
