package db

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repo is the DI adapter over the settings table. Construct at the composition
// root and pass to the settings Module instead of dialing the global
// connection.
type Repo struct {
	db *gorm.DB
}

func NewRepo(conn *gorm.DB) *Repo { return &Repo{db: conn} }

// Get returns the stored value for key. found=false with a nil error means no
// row exists — the caller must fall back to the env/compiled default rather
// than treat the empty string as an intentional override.
func (r *Repo) Get(key string) (value string, found bool, err error) {
	var s Setting
	err = r.db.Where("key = ?", key).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return s.Value, true, nil
}

// Set upserts the value for key. UpdatedAt is written explicitly because the
// ON CONFLICT update path does not run GORM's autoUpdateTime hook.
func (r *Repo) Set(key, value string) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&Setting{Key: key, Value: value, UpdatedAt: time.Now()}).Error
}

// GetAll returns every stored override, ordered by key for stable output.
func (r *Repo) GetAll() ([]Setting, error) {
	var rows []Setting
	return rows, r.db.Order("key ASC").Find(&rows).Error
}

// Delete removes the override for key. Idempotent: deleting a missing key is
// not an error — the effective value simply reverts to the env/compiled
// default on the next read/hydrate.
func (r *Repo) Delete(key string) error {
	return r.db.Where("key = ?", key).Delete(&Setting{}).Error
}
