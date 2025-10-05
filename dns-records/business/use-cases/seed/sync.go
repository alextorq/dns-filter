package seed

import (
	blacklists "github.com/alextorq/dns-filter/dns-records"
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
		l.Info("There are %d records in the database. Skip loading from dns-records.", amount)
	}
	return nil
}
