package db

import (
	"time"

	"github.com/alextorq/dns-filter/db"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repo is the output adapter for the unified per-device traffic counter table.
// Construct with NewRepo(gormDB) in main and pass it to consumers instead of
// reaching for db.GetConnection() package-level state — same convention as
// blocked-domain/db.Repo.
type Repo struct {
	db *gorm.DB
}

func NewRepo(conn *gorm.DB) *Repo {
	return &Repo{db: conn}
}

// upsertBatchSize bounds rows per INSERT so we stay under SQLite's 32766
// bound-parameter limit. DomainTraffic binds 8 columns/row
// (client_kind, client_value, client_ip, domain, blocked, day, count, last_seen;
// the auto id is generated, not bound) — 4000 × 8 = 32000 < 32766, keeping
// headroom and matching the batch size used for block_lists inserts.
const upsertBatchSize = 4000

// UpsertBatch additively upserts the rows. On conflict of the unique key
// (client_kind, client_value, blocked, domain, day) the existing row is updated:
//
//	count     = count + excluded.count        (accumulate, never overwrite)
//	last_seen = max(last_seen, excluded.last_seen)  (never roll the clock back)
//	client_ip = excluded.client_ip            (latest IP wins — informational)
//
// Empty input is a no-op. Rows are committed per batch (see db.batchOn) to avoid
// holding SQLite's single write lock long enough to starve the async recorder.
func (r *Repo) UpsertBatch(rows []DomainTraffic) error {
	if len(rows) == 0 {
		return nil
	}
	onConflict := clause.OnConflict{
		Columns: []clause.Column{
			{Name: "client_kind"},
			{Name: "client_value"},
			{Name: "blocked"},
			{Name: "domain"},
			{Name: "day"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"count":     gorm.Expr("count + excluded.count"),
			"last_seen": gorm.Expr("MAX(last_seen, excluded.last_seen)"),
			"client_ip": gorm.Expr("excluded.client_ip"),
		}),
	}
	return db.BatchUpsertWith(r.db, rows, upsertBatchSize, onConflict)
}

// DeleteOlderThan hard-deletes every row whose Day is strictly before cutoff.
// Unscoped because DomainTraffic has no soft-delete column; a row on the cutoff
// day itself is kept (strict <). It is the daily retention prune that replaces
// the two legacy clear-events tasks. An empty table is a harmless no-op.
func (r *Repo) DeleteOlderThan(cutoff time.Time) error {
	return r.db.Unscoped().
		Where("day < ?", cutoff).
		Delete(&DomainTraffic{}).Error
}
