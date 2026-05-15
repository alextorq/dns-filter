package db

import (
	"gorm.io/gorm"
)

// Repo is the DI adapter over allow_domain_events. Construct at the
// composition root and pass to consumers (suggest-to-block) instead of
// dialing the package-level helper that reads the global connection.
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
