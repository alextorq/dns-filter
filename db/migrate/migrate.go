package migrate

import (
	allow_domain_db "github.com/alextorq/dns-filter/allow-domain/db"
	auth_db "github.com/alextorq/dns-filter/auth/db"
	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	exclude_clients "github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/db"
	syncDb "github.com/alextorq/dns-filter/source/db"
	suggest_db "github.com/alextorq/dns-filter/suggest-to-block/db"
)

func Migrate() {
	connect := db.GetConnection()

	// One-shot reset of suggest tables when the legacy schema is detected.
	// Pre-refactor SuggestBlock had a `reason` TEXT column with concatenated
	// human-readable strings; the new schema replaces it with a normalized
	// suggest_block_reasons table. Backfilling string→code is brittle, so
	// we drop both tables and let the next Collect() tick (runs at startup)
	// rebuild rows with proper codes.
	//
	// Gated on HasColumn so this fires exactly once: after the drop,
	// AutoMigrate recreates suggest_blocks without `reason` and the check
	// is false on every subsequent boot.
	m := connect.Migrator()
	if m.HasTable(&suggest_db.SuggestBlock{}) &&
		m.HasColumn(&suggest_db.SuggestBlock{}, "reason") {
		if err := m.DropTable(&suggest_db.SuggestBlockReason{}, &suggest_db.SuggestBlock{}); err != nil {
			panic(err)
		}
	}

	err := connect.AutoMigrate(
		&suggest_db.SuggestBlock{},
		&suggest_db.SuggestBlockReason{},
		&blocked_domain_db.BlockList{},
		&blocked_domain_db.BlockDomainEvent{},
		&allow_domain_db.AllowDomainEvent{},
		&exclude_clients.ExcludeClient{},
		&syncDb.Source{},
		&auth_db.User{},
		&auth_db.Session{},
	)
	if err != nil {
		panic(err)
	}
}
