package db

import (
	"errors"

	create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	"github.com/alextorq/dns-filter/db"
	"github.com/alextorq/dns-filter/utils"
	"gorm.io/gorm"
)

// Repo is the output adapter that talks to SQLite via GORM. Use-cases depend on
// narrow interfaces (output ports) and Repo's methods satisfy them via Go's
// structural typing. See blocked-domain/ports.go for the consumer-side ports.
//
// Construct with NewRepo(gormDB) in main and pass it everywhere instead of
// reading db.GetConnection() from package-level state.
type Repo struct {
	db *gorm.DB
}

func NewRepo(conn *gorm.DB) *Repo {
	return &Repo{db: conn}
}

// ----- BlockList read -----

func (r *Repo) GetByID(id uint) (*BlockList, error) {
	var rec BlockList
	if err := r.db.Where("id = ?", id).First(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *Repo) GetRecordsByFilter(filter GetAllParams) (GetRecordsResult, error) {
	var lists []BlockList
	query := r.db.Model(&BlockList{})
	var total int64

	if filter.Filter != "" {
		query = query.Where("url LIKE ?", "%"+filter.Filter+"%")
	}
	if filter.Source != "" {
		query = query.Where("source = ?", filter.Source)
	}

	query.Count(&total)

	if filter.Filter != "" {
		// Релевантность: точное совпадение → искомый домен как поддомен
		// (суффикс по точке) → префикс → произвольная подстрока. Внутри тира
		// короче и алфавитно — выше. relevance не маппится в BlockList и
		// нужен только для ORDER BY.
		query = query.Select(
			"*, CASE"+
				" WHEN url = ? THEN 0"+
				" WHEN url LIKE ? THEN 1"+
				" WHEN url LIKE ? THEN 2"+
				" ELSE 3 END AS relevance",
			filter.Filter, "%."+filter.Filter, filter.Filter+"%",
		).Order("relevance, LENGTH(url), url")
	}

	// Preload reasons so AutoBlocked rows expose *why* they were promoted (#95).
	// Source rows have none — the join returns nothing extra for those.
	err := query.Preload("Reasons").Limit(filter.Limit).Offset(filter.Offset).Find(&lists).Error
	return GetRecordsResult{Total: total, List: lists}, err
}

func (r *Repo) GetAllActiveURLs() ([]string, error) {
	var urls []string
	err := r.db.Model(&BlockList{}).
		Where("active = ?", true).
		Pluck("url", &urls).Error
	if err != nil {
		return nil, err
	}
	return urls, nil
}

// DomainNotExist reports whether the domain has no row in the block list.
//
// Invariant: domain must already be in canonical form (utils.CanonicalDomain) —
// the `url` column stores only canonical FQDNs, so a non-canonical argument
// would always look absent. Callers go through create_domain.CreateDomain,
// which canonicalizes first (#30).
func (r *Repo) DomainNotExist(domain string) bool {
	var rec BlockList
	err := r.db.Where("url = ?", domain).First(&rec).Error
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// IsActivelyBlocked reports whether the domain has an active record in the
// block list. Errors must propagate: caching "not blocked" on a transient DB
// failure would silently disable filtering for the LRU window (issue #25).
//
// Invariant: domain must already be canonical (utils.CanonicalDomain) — the
// hot path passes it through filter.Module.CheckExist, which canonicalizes
// the miekg/dns query name first (#30).
func (r *Repo) IsActivelyBlocked(domain string) (bool, error) {
	var count int64
	if err := r.db.Model(&BlockList{}).
		Where("url = ? AND active = ?", domain, true).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// ----- BlockList write -----

func (r *Repo) CreateDomain(domain, source string) error {
	rec := BlockList{Url: domain, Active: true, Source: source}
	return r.db.Create(&rec).Error
}

// CreateDomainWithReasons writes the block-list row together with its reason
// rows. GORM's default Create wraps the row and its associations in a single
// transaction, so a failed reason insert rolls back the block_lists row too —
// a domain never lands on the list without the reasons it was promoted for
// (#95). Callers must have already checked DomainNotExist; this is the
// auto-block path of create_domain.CreateDomain.
func (r *Repo) CreateDomainWithReasons(domain, source string, reasons []create_domain.Reason) error {
	rec := BlockList{Url: domain, Active: true, Source: source}
	rec.Reasons = make([]BlockListReason, 0, len(reasons))
	for _, rs := range reasons {
		rec.Reasons = append(rec.Reasons, BlockListReason{Code: rs.Code, MatchValue: rs.Match})
	}
	return r.db.Create(&rec).Error
}

func (r *Repo) UpdateBlockList(rec *BlockList) error {
	return r.db.Save(rec).Error
}

func (r *Repo) CreateDNSRecordsByDomains(urls []string, source string) error {
	if len(urls) == 0 {
		return nil
	}
	deduped := utils.OnlyUniqString(urls)
	entries := make([]BlockList, 0, len(deduped))
	for _, u := range deduped {
		entries = append(entries, BlockList{Url: u, Active: true, Source: source})
	}
	// SQLite parameter limit is 32766 (3.32+). BlockList writes 7 columns
	// (id, created_at, updated_at, deleted_at, url, active, source) — 5000
	// rows × 7 ≈ 35k. 4000 keeps headroom.
	return db.BatchUpsertOn(r.db, entries, 4000)
}

// idURL is a lightweight projection of (block_lists.id, block_lists.url) for
// methods that diff the table in memory.
type idURL struct {
	ID  uint
	Url string
}

// staleDeleteBatch bounds the `id IN (...)` / `domain_id IN (...)` lists when
// pruning vanished domains. SQLite caps bound parameters at 32766; 4000 keeps
// a wide margin and matches the batch size used on the insert side.
const staleDeleteBatch = 4000

// DeleteDNSRecordsBySourceNotIn hard-deletes block_lists rows of source whose
// url is absent from keep. Deletion is scoped strictly to source, so User /
// AutoBlocked / SuggestedToBlock rows are never touched.
//
// keep is the union of every freshly synced source, not just this one: a
// domain shared by two block lists must survive as long as any list still
// carries it (see source.Sync). An empty keep is a no-op — pruning against
// nothing would wipe the source, and an empty fresh set means a failed sync.
//
// The stale set is diffed in memory (a `NOT IN` over the full keep set would
// blow past SQLite's bound-parameter limit) and removed in batches inside one
// transaction.
func (r *Repo) DeleteDNSRecordsBySourceNotIn(source string, keep []string) error {
	if len(keep) == 0 {
		return nil
	}
	keepSet := make(map[string]struct{}, len(keep))
	for _, u := range keep {
		keepSet[u] = struct{}{}
	}

	var rows []idURL
	if err := r.db.Model(&BlockList{}).
		Where("source = ?", source).
		Select("id", "url").Find(&rows).Error; err != nil {
		return err
	}

	staleIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		if _, ok := keepSet[row.Url]; !ok {
			staleIDs = append(staleIDs, row.ID)
		}
	}
	if len(staleIDs) == 0 {
		return nil
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < len(staleIDs); i += staleDeleteBatch {
			batch := staleIDs[i:min(i+staleDeleteBatch, len(staleIDs))]
			if err := tx.Unscoped().
				Where("id IN ?", batch).
				Delete(&BlockList{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repo) ChangeRecordStatusBySource(source string, active bool) error {
	return r.db.Model(&BlockList{}).
		Where("source = ?", source).
		Update("active", active).Error
}
