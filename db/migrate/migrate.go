package migrate

import (
	"github.com/alextorq/dns-filter/blocked-domain"
	"github.com/alextorq/dns-filter/db"
	blacklists "github.com/alextorq/dns-filter/dns-records"
)

func Migrate() {
	connect := db.GetConnection()
	err := connect.AutoMigrate(
		&blacklists.BlockList{},
		&blocked_domain.BlockDomainEvent{},
	)
	if err != nil {
		panic(err)
	}
}
