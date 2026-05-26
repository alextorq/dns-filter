package migrate

import (
	auth_db "github.com/alextorq/dns-filter/auth/db"
	blocked_domain_db "github.com/alextorq/dns-filter/blocked-domain/db"
	clients_db "github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/db"
	settings_db "github.com/alextorq/dns-filter/settings/db"
	syncDb "github.com/alextorq/dns-filter/source/db"
	suggest_db "github.com/alextorq/dns-filter/suggest-to-block/db"
	traffic_db "github.com/alextorq/dns-filter/traffic/db"
	"gorm.io/gorm"
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
		&blocked_domain_db.BlockListReason{},
		&clients_db.Client{},
		&syncDb.Source{},
		&auth_db.User{},
		&auth_db.Session{},
		&settings_db.Setting{},
		&traffic_db.DomainTraffic{},
	)
	if err != nil {
		panic(err)
	}

	// Drop the two legacy event tables. They were dual-written until the
	// per-device traffic counter (domain_traffic) fully replaced them; the
	// unified table now serves block stats, the suggest candidate pool, and
	// domain-inspect allow membership. No backfill — old per-day rows are
	// simply discarded (see TRAFFIC_DASHBOARD_PLAN.md Step 7). DropTable is a
	// no-op once the tables are gone, so this is idempotent across boots.
	for _, table := range []string{"block_domain_events", "allow_domain_events"} {
		if m.HasTable(table) {
			if err := m.DropTable(table); err != nil {
				panic(err)
			}
		}
	}

	// One-shot migration from the legacy exclude_clients table to clients.
	// The old schema stored {user_id, active}: active=true meant "filter is
	// bypassed for this IP". The new model inverts the semantic into
	// Filtered (true=filter applies, false=excluded), so an old active=true
	// row maps to a new Filtered=false row. Rows with active=false are kept
	// as Filtered=true so the user doesn't lose the IPs they previously
	// disabled — they appear in the new UI as "filtered normally".
	//
	// The HasTable check makes this a no-op after the table is dropped, so
	// subsequent boots skip it entirely. Fresh installs (where the legacy
	// table never existed) fall through to AutoMigrate above and stop here.
	//
	// The clients-non-empty guard is the partial-failure escape hatch: if a
	// previous boot copied rows but crashed before DropTable could finish,
	// the next boot would otherwise duplicate every row. Skipping the copy
	// when clients already has data keeps re-runs idempotent.
	//
	// The copy itself runs inside a transaction so a mid-batch failure
	// (constraint violation, disk full, etc.) rolls back rather than leaving
	// a partially-populated clients table — which would falsely satisfy the
	// non-empty guard on the next boot and silently lose the rest of the
	// legacy data when DropTable runs.
	if m.HasTable("exclude_clients") {
		var clientsCount int64
		if err := connect.Model(&clients_db.Client{}).Count(&clientsCount).Error; err != nil {
			panic(err)
		}
		if clientsCount == 0 {
			err := connect.Transaction(func(tx *gorm.DB) error {
				return migrateExcludeClients(tx)
			})
			if err != nil {
				panic(err)
			}
		}
		if err := m.DropTable("exclude_clients"); err != nil {
			panic(err)
		}
	}
}

// migrateExcludeClients copies rows from the legacy exclude_clients table
// into the new clients table. Soft-deleted rows are skipped — they were
// invisible in the old UI and resurrecting them now would be surprising.
func migrateExcludeClients(con *gorm.DB) error {
	type legacyRow struct {
		UserId string
		Active bool
	}
	var rows []legacyRow
	if err := con.Table("exclude_clients").
		Where("deleted_at IS NULL").
		Select("user_id", "active").
		Scan(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		c := clients_db.Client{
			IP:       r.UserId,
			Filtered: !r.Active,
		}
		if err := con.Create(&c).Error; err != nil {
			return err
		}
	}
	return nil
}
