package migrate

import (
	allow_domain "github.com/alextorq/dns-filter/allow-domain"
	db2 "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/db"
	blacklists "github.com/alextorq/dns-filter/dns-records"
)

func Migrate() {
	connect := db.GetConnection()
	err := connect.AutoMigrate(
		&blacklists.BlockList{},
		&db2.BlockDomainEvent{},
		&allow_domain.AllowDomainEvent{},
	)
	if err != nil {
		panic(err)
	}
}
