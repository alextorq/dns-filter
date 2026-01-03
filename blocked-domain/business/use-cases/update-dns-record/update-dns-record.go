package update_dns_record

import (
	"fmt"

	"github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/logger"
)

type UpdateBlockList struct {
	ID     uint `json:"id" binding:"required"`
	Active bool `json:"active"`
}

func UpdateDnsRecord(update UpdateBlockList) (*db.BlockList, error) {
	l := logger.GetLogger()

	record, err := db.GetBlockListByID(update.ID)

	if err != nil {
		wrap := fmt.Errorf("error get record by id when change record: %w", err)
		l.Error(wrap)
		return nil, wrap
	}

	record.Active = update.Active

	err = record.Update()
	if err != nil {
		wrap := fmt.Errorf("error update record when change record: %w", err)
		l.Error(wrap)
		return nil, wrap
	} else {
		l.Info("Record updated:", record)
	}

	return record, nil
}
