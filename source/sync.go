package source

import (
	"github.com/alextorq/dns-filter/db"
	syncRec "github.com/alextorq/dns-filter/source/business/use-cases/sync"
)
import "github.com/alextorq/dns-filter/source/business/use-cases/seed"
import syncDb "github.com/alextorq/dns-filter/source/db"

func Sync() error {
	connect := db.GetConnection()
	seed.SeedSyncs(connect)
	return syncRec.Sync()
}

func GetAllRecords() ([]syncDb.Source, error) {
	return syncDb.GetAllRecords(syncDb.GetAllParams{})
}
