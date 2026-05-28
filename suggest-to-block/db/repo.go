package db

import (
	"gorm.io/gorm"
)

// Repo is the DI adapter over the suggest_blocks tables. Construct one at the
// composition root with NewRepo(conn) and pass it to consumers (the suggest
// module and HTTP handlers) instead of dialing the package-level helpers.
type Repo struct {
	db *gorm.DB
}

func NewRepo(conn *gorm.DB) *Repo { return &Repo{db: conn} }

func (r *Repo) CreateBatch(suggests []SuggestBlock) error {
	return createSuggestBlockBatchOn(r.db, suggests)
}

func (r *Repo) GetByFilter(params GetAllParams) (*GetAllResult, error) {
	return getAllSuggestBlocksOn(r.db, params)
}

func (r *Repo) UpdateActive(id uint, active bool) error {
	return r.db.Model(&SuggestBlock{}).Where("id = ?", id).Update("active", active).Error
}

// UpsertWithInspect promotes a domain from the inspect queue into the suggest
// list, creating it or refreshing only its inspect_* reasons. See
// upsertWithInspectOn for the full contract.
func (r *Repo) UpsertWithInspect(domain string, lexicalScore int, reasons []SuggestBlockReason) error {
	return upsertWithInspectOn(r.db, domain, lexicalScore, reasons)
}
