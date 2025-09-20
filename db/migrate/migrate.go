package migrate

import (
	blacklists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/db"
	"github.com/alextorq/dns-filter/events"
)

func Migrate() {
	connect := db.GetConnection()
	err := connect.AutoMigrate(
		&blacklists.BlockList{},
		&events.BlockDomainEvent{},
	)
	if err != nil {
		panic(err)
	}
}
