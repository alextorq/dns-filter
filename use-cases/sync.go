package use_cases

import (
	blacklists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/logger"
)

func Sync() error {
	l := logger.GetLogger()
	amount := blacklists.GetAmountRecords()
	if amount == 0 {
		list := blacklists.LoadAll()
		err := blacklists.CreateDNSRecordsByDomains(list)
		return err
	} else {
		l.Info("There are %d records in the database. Skip loading from black-lists.", amount)
	}
	return nil
}
