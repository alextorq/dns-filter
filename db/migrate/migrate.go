package migrate

import (
	blacklists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/db"
)

func Migrate() {
	connect := db.GetConnection()
	err := connect.AutoMigrate(
		&blacklists.BlockList{},
	)
	if err != nil {
		panic(err)
	}
}
