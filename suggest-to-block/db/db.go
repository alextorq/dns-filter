package db

import (
	"strings"

	"gorm.io/gorm"
)

// InspectReasonPrefix marks reason codes produced by the reputation worker (as
// opposed to the lexical pass). It is the discriminator UpsertWithInspect uses
// to refresh only worker-derived reasons on a re-run while leaving the lexical
// ones intact. No lexical code (see collect package) may start with it.
const InspectReasonPrefix = "inspect_"

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

// upsertWithInspectOn promotes a domain from the inspect queue into the suggest
// list. `reasons` is the combined set (lexical snapshot + inspect_* codes), all
// as SuggestBlockReason. Score stays lexical.
//
//   - New domain: insert the row with all reasons.
//   - Existing domain: keep the lexical reasons untouched, replace only the
//     inspect_* ones (idempotent across worker cycles), and refresh the score.
//     Active is deliberately NOT reset — if an operator deactivated the row, a
//     later worker pass must not silently resurrect it.
func upsertWithInspectOn(conn *gorm.DB, domain string, lexicalScore int, reasons []SuggestBlockReason) error {
	return conn.Transaction(func(tx *gorm.DB) error {
		// Find (not First) so a missing row is RowsAffected==0 rather than an
		// ErrRecordNotFound the GORM logger prints at error level on every insert.
		var existing SuggestBlock
		found := tx.Where("domain = ?", domain).Limit(1).Find(&existing)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected == 0 {
			return tx.Create(&SuggestBlock{
				Domain:  domain,
				Score:   lexicalScore,
				Reasons: reasons,
			}).Error
		}

		if err := tx.Model(&existing).Update("score", lexicalScore).Error; err != nil {
			return err
		}

		// Delete existing inspect_* reasons by id. We match the prefix in Go
		// rather than via SQL LIKE 'inspect_%' because '_' is a single-char
		// wildcard in LIKE — a literal underscore there would over-match.
		var current []SuggestBlockReason
		if err := tx.Where("suggest_id = ?", existing.ID).Find(&current).Error; err != nil {
			return err
		}
		var staleIDs []uint
		for _, r := range current {
			if strings.HasPrefix(r.Code, InspectReasonPrefix) {
				staleIDs = append(staleIDs, r.ID)
			}
		}
		if len(staleIDs) > 0 {
			if err := tx.Delete(&SuggestBlockReason{}, staleIDs).Error; err != nil {
				return err
			}
		}

		// Re-insert only the inspect_* reasons from the incoming set; the lexical
		// ones are already attached from the original insert.
		var toAdd []SuggestBlockReason
		for _, r := range reasons {
			if strings.HasPrefix(r.Code, InspectReasonPrefix) {
				toAdd = append(toAdd, SuggestBlockReason{
					SuggestID:  existing.ID,
					Code:       r.Code,
					MatchValue: r.MatchValue,
				})
			}
		}
		if len(toAdd) > 0 {
			return tx.Create(&toAdd).Error
		}
		return nil
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

	// По умолчанию подозрительные домены идут по убыванию score. Но при поиске
	// по строке релевантность важнее: точное совпадение → искомый домен как
	// поддомен (суффикс по точке) → префикс → произвольная подстрока; внутри
	// тира сохраняется сортировка по score. relevance не маппится в
	// SuggestBlock и нужен только для ORDER BY.
	order := "score DESC, id DESC"
	if params.Filter != "" {
		query = query.Select(
			"*, CASE"+
				" WHEN domain = ? THEN 0"+
				" WHEN domain LIKE ? THEN 1"+
				" WHEN domain LIKE ? THEN 2"+
				" ELSE 3 END AS relevance",
			params.Filter, "%."+params.Filter, params.Filter+"%",
		)
		order = "relevance, " + order
	}

	err := query.
		Preload("Reasons").
		Order(order).
		Limit(params.Limit).
		Offset(params.Offset).
		Find(&suggests).
		Error

	return &GetAllResult{
		List:  suggests,
		Total: total,
	}, err
}
