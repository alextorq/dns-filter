package db

import (
	"time"

	app_db "github.com/alextorq/dns-filter/db"
	"gorm.io/gorm"
)

// Repo is the DI adapter over allow_domain_events. Construct at the
// composition root and pass to consumers (suggest-to-block, the allow event
// store, the cleanup task) instead of dialing the package-level helper that
// reads the global connection.
type Repo struct {
	db *gorm.DB
}

func NewRepo(conn *gorm.DB) *Repo { return &Repo{db: conn} }

func (r *Repo) GetAllActiveFilters() ([]string, error) {
	var domains []string
	err := r.db.Model(&AllowDomainEvent{}).
		Where("active = ?", true).
		Pluck("domain", &domains).Error
	if err != nil {
		return nil, err
	}
	return domains, nil
}

// CreateBatch upserts the given domains as active allow entries. Duplicate
// domains in the input — or domains already present in the table — are
// silently ignored via the unique index on Domain. Empty input is a no-op.
func (r *Repo) CreateBatch(domains []string) error {
	if len(domains) == 0 {
		return nil
	}
	events := make([]AllowDomainEvent, 0, len(domains))
	for _, d := range domains {
		events = append(events, AllowDomainEvent{Domain: d, Active: true})
	}
	return app_db.BatchUpsertOn(r.db, events, 0)
}

// DeleteOlderThan permanently deletes allow_domain_events whose CreatedAt is
// more than `days` days in the past. Hard-delete (Unscoped) so the cleanup
// task actually frees the row instead of just setting deleted_at.
func (r *Repo) DeleteOlderThan(days int) error {
	cutoff := time.Now().AddDate(0, 0, -days)
	return r.db.Unscoped().
		Where("created_at < ?", cutoff).
		Delete(&AllowDomainEvent{}).Error
}
