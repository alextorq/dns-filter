package migrate

import (
	blacklists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/db"
)

func Migrate() {
	connect := db.GetConnection()
	err := connect.AutoMigrate(
		&blacklists.BlockList{},
		&blacklists.BlockDomain{},
	)
	if err != nil {
		panic(err)
	}
}
