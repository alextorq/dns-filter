package migrate

import (
	allow_domain_db "github.com/alextorq/dns-filter/allow-domain/db"
	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/db"
	dns_records_db "github.com/alextorq/dns-filter/dns-records/db"
)

func Migrate() {
	connect := db.GetConnection()
	err := connect.AutoMigrate(
		&dns_records_db.BlockList{},
		&blocked_domain_db.BlockDomainEvent{},
		&allow_domain_db.AllowDomainEvent{},
	)
	if err != nil {
		panic(err)
	}
}
