package update_dns_record

import (
	"fmt"

	blacklists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/logger"
	"github.com/alextorq/dns-filter/use-cases"
)

type UpdateBlockList struct {
	ID     uint `json:"id" binding:"required"`
	Active bool `json:"active"`
}

func UpdateDnsRecord(update UpdateBlockList) (*blacklists.BlockList, error) {
	l := logger.GetLogger()

	record, err := blacklists.GetBlockListByID(update.ID)

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

	err = use_cases.UpdateFilterFromDb()
	if err != nil {
		wrap := fmt.Errorf("error update filter from db when change record: %w", err)
		l.Error(wrap)
		return nil, wrap
	}

	return record, err
}
