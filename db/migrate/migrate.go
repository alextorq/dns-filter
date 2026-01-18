package migrate

import (
	allow_domain_db "github.com/alextorq/dns-filter/allow-domain/db"
	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	"github.com/alextorq/dns-filter/db"
	syncDb "github.com/alextorq/dns-filter/source/db"
	suggest_db "github.com/alextorq/dns-filter/suggest-to-block/db"
)

func Migrate() {
	connect := db.GetConnection()
	err := connect.AutoMigrate(
		&suggest_db.SuggestBlock{},
		&blocked_domain_db.BlockList{},
		&blocked_domain_db.BlockDomainEvent{},
		&allow_domain_db.AllowDomainEvent{},
		&syncDb.Source{},
	)
	if err != nil {
		panic(err)
	}
}
