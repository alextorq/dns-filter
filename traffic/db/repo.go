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

// GetAllowedDomains returns the DISTINCT set of domains ever seen with a
// blocked=false verdict (i.e. forwarded upstream). It is the suggest-to-block
// candidate pool, replacing the allow-domain repo's GetAllActiveFilters: a
// domain that real clients keep resolving is the input to the heuristic
// scorer. A domain that was only ever blocked is excluded — it is not a
// candidate to block again. Deduped across days and devices by SELECT DISTINCT.
// An empty table returns an empty (non-nil) slice.
func (r *Repo) GetAllowedDomains() ([]string, error) {
	domains := []string{}
	err := r.db.Model(&DomainTraffic{}).
		Where("blocked = ?", false).
		Distinct().
		Pluck("domain", &domains).Error
	if err != nil {
		return nil, err
	}
	return domains, nil
}

// IsAllowed reports whether the domain has ever been forwarded upstream
// (a row with blocked=false). It is verdict-scoped: a domain seen only as
// blocked is NOT "allowed". Backs domain-inspect's allow-membership signal,
// replacing the allow_domain_events lookup.
func (r *Repo) IsAllowed(domain string) (bool, error) {
	return r.exists(domain, false)
}

// IsBlockedSeen reports whether the domain has ever been NXDOMAIN'd (a row with
// blocked=true). Verdict-scoped, mirror of IsAllowed. Provided for symmetry /
// future use — domain-inspect's block-membership signal still reads the
// block_lists table directly, so this is not wired into a consumer in Step 3.
func (r *Repo) IsBlockedSeen(domain string) (bool, error) {
	return r.exists(domain, true)
}

// exists is the shared membership check for the verdict-scoped helpers; n>0 is
// the EXISTS answer. The (domain, blocked) predicate is not a left-prefix of the
// composite unique index, so this is a scan — negligible since domain-inspect
// calls it once per inspect. Add a (domain, blocked) index if profiling warrants.
func (r *Repo) exists(domain string, blocked bool) (bool, error) {
	var n int64
	err := r.db.Model(&DomainTraffic{}).
		Where("domain = ? AND blocked = ?", domain, blocked).
		Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
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
