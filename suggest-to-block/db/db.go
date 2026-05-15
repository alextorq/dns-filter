package db

import (
	"gorm.io/gorm"
)

type SuggestBlock struct {
	ID      uint                 `gorm:"primarykey" json:"id"`
	Domain  string               `json:"domain" gorm:"uniqueIndex"`
	Score   int                  `json:"score"`
	Active  bool                 `gorm:"default:true" json:"active"`
	Reasons []SuggestBlockReason `gorm:"foreignKey:SuggestID;constraint:OnDelete:CASCADE" json:"reasons"`
}

type SuggestBlockReason struct {
	ID         uint   `gorm:"primarykey" json:"id"`
	SuggestID  uint   `gorm:"index;not null" json:"-"`
	Code       string `gorm:"index;not null" json:"code"`
	MatchValue string `json:"match,omitempty"`
}

// createSuggestBlockBatchOn inserts only suggestions whose Domain is not yet
// stored — preserving the previous "do nothing on conflict" semantics.
// The reasons attached to a Suggestion are inserted via GORM associations
// in the same transaction, so existing rows keep their original reasons
// and never accumulate duplicates across collector runs.
func createSuggestBlockBatchOn(conn *gorm.DB, suggests []SuggestBlock) error {
	if len(suggests) == 0 {
		return nil
	}
	return conn.Transaction(func(tx *gorm.DB) error {
		domains := make([]string, 0, len(suggests))
		for _, s := range suggests {
			domains = append(domains, s.Domain)
		}

		var existing []string
		if err := tx.Model(&SuggestBlock{}).
			Where("domain IN ?", domains).
			Pluck("domain", &existing).Error; err != nil {
			return err
		}

		taken := make(map[string]struct{}, len(existing))
		for _, d := range existing {
			taken[d] = struct{}{}
		}

		toCreate := make([]SuggestBlock, 0, len(suggests))
		for _, s := range suggests {
			if _, ok := taken[s.Domain]; ok {
				continue
			}
			toCreate = append(toCreate, s)
		}
		if len(toCreate) == 0 {
			return nil
		}
		return tx.CreateInBatches(toCreate, 100).Error
	})
}

type GetAllParams struct {
	Limit  int
	Offset int
	Filter string
	Active *bool
	// Codes filters by reason codes (OR semantic). Empty/nil = no filter.
	// Implemented via EXISTS over suggest_block_reasons so Reasons preload
	// still returns ALL reasons of a matched suggest, not just the matched ones.
	Codes []string
}

type GetAllResult struct {
	List  []SuggestBlock `json:"list"`
	Total int64          `json:"total"`
}

func getAllSuggestBlocksOn(conn *gorm.DB, params GetAllParams) (*GetAllResult, error) {
	var suggests []SuggestBlock
	query := conn.Model(&SuggestBlock{})
	var total int64

	// если нужно фильтровать по строке
	if params.Filter != "" {
		query = query.Where("domain LIKE ?", "%"+params.Filter+"%")
	}

	// если передан параметр Active, фильтруем по нему
	if params.Active != nil {
		query = query.Where("active = ?", *params.Active)
	}

	// Filter by reason codes (OR): keep suggests that have at least one
	// reason whose code is in the requested set.
	if len(params.Codes) > 0 {
		query = query.Where(
			"EXISTS (SELECT 1 FROM suggest_block_reasons r WHERE r.suggest_id = suggest_blocks.id AND r.code IN ?)",
			params.Codes,
		)
	}

	// сначала считаем количество
	query.Count(&total)

	err := query.
		Preload("Reasons").
		Order("score DESC, id DESC").
		Limit(params.Limit).
		Offset(params.Offset).
		Find(&suggests).
		Error

	return &GetAllResult{
		List:  suggests,
		Total: total,
	}, err
}
