package use_cases

import (
	"time"

	black_lists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/db"
)

func StartCleanUpBlockDomain() {
	conn := db.GetConnection()
	black_lists.StartBlockDomainCleanup(conn, 100000, 10*time.Minute)
}
