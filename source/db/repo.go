package db

import (
	"errors"

	"gorm.io/gorm"
)

// Repo is the DI adapter over the sources table. Construct at the composition
// root and pass to consumers instead of dialing package-level helpers that
// read the global connection.
type Repo struct {
	db *gorm.DB
}

func NewRepo(conn *gorm.DB) *Repo { return &Repo{db: conn} }

func (r *Repo) GetAll(filter GetAllParams) ([]Source, error) {
	var records []Source
	query := r.db.Model(&Source{})
	if filter.Filter != "" {
		query = query.Where("name LIKE ?", "%"+filter.Filter+"%")
	}
	return records, query.Find(&records).Error
}

func (r *Repo) GetAllActive() ([]Source, error) {
	var records []Source
	return records, r.db.Model(&Source{}).Where("active = true").Find(&records).Error
}

func (r *Repo) Amount() int64 {
	var count int64
	r.db.Model(&Source{}).Count(&count)
	return count
}

func (r *Repo) GetByID(id uint) (*Source, error) {
	var s Source
	if err := r.db.Where("id = ?", id).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repo) Update(s *Source) error {
	return r.db.Save(s).Error
}

// Seed inserts the default catalog of known sources if they are missing.
// Idempotent (FirstOrCreate) — safe to call on every startup.
func (r *Repo) Seed() {
	defaults := []Source{
		{Name: SourceStevenBlack, Active: true},
		{Name: SourceEasyList, Active: true},
		{Name: SourceRuAdList, Active: true},
		{Name: SourceAdGuardRussian, Active: true},
		{Name: SourceHaGeZiMulti, Active: true},
		{Name: SourceUser, Active: true},
		{Name: SourceSuggestedToBlock, Active: true},
		{Name: SourceAutoBlocked, Active: true},
	}
	for _, item := range defaults {
		if err := r.db.FirstOrCreate(&item, Source{Name: item.Name}).Error; err != nil {
			println("Ошибка при создании дефолтной записи:", item.Name, err.Error())
		}
	}
}

// IsActive reports whether the named source is currently enabled. Missing
// row → false (fail-closed): startup Seed guarantees every known source has a
// row, so an absent one means the DB is in an unknown state and we'd rather
// skip the auto-promotion than silently re-enable a kill-switch the operator
// may have disabled. Same logic for DB errors — callers must surface them and
// treat as "not active" defensively.
func (r *Repo) IsActive(name BlockListSource) (bool, error) {
	var s Source
	err := r.db.Where("name = ?", name).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return s.Active, nil
}
