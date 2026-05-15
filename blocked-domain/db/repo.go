package db

import (
	"errors"
	"time"

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

	err := query.Limit(filter.Limit).Offset(filter.Offset).Find(&lists).Error
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

func (r *Repo) DomainNotExist(domain string) bool {
	var rec BlockList
	err := r.db.Where("url = ?", domain).First(&rec).Error
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// IsActivelyBlocked reports whether the domain has an active record in the
// block list. Errors must propagate: caching "not blocked" on a transient DB
// failure would silently disable filtering for the LRU window (issue #25).
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

func (r *Repo) ChangeRecordStatusBySource(source string, active bool) error {
	return r.db.Model(&BlockList{}).
		Where("source = ?", source).
		Update("active", active).Error
}

// ----- BlockDomainEvent -----

func (r *Repo) BatchCreateBlockDomainEvents(domains []string) error {
	if len(domains) == 0 {
		return nil
	}
	uniq := utils.OnlyUniqString(domains)
	type idURL struct {
		ID  uint
		Url string
	}
	var rows []idURL
	if err := r.db.Model(&BlockList{}).
		Select("id", "url").
		Where("url IN ?", uniq).
		Find(&rows).Error; err != nil {
		return err
	}
	domainMap := make(map[string]uint, len(rows))
	for _, row := range rows {
		domainMap[row.Url] = row.ID
	}
	var events []BlockDomainEvent
	for _, d := range domains {
		if id, ok := domainMap[d]; ok {
			events = append(events, BlockDomainEvent{DomainId: id})
		}
	}
	return db.BatchInsertOn(r.db, events, 100)
}

func (r *Repo) DeleteEventsOlderThan(days int) error {
	cutoff := time.Now().AddDate(0, 0, -days)
	return r.db.Unscoped().
		Where("created_at < ?", cutoff).
		Delete(&BlockDomainEvent{}).Error
}

func (r *Repo) GetEventsAmount() int64 {
	var count int64
	r.db.Model(&BlockDomainEvent{}).Count(&count)
	return count
}

func (r *Repo) GetEventsByDomain() ([]DomainCount, error) {
	var results []DomainCount
	err := r.db.Model(&BlockDomainEvent{}).
		Select("block_lists.url as domain, COUNT(block_domain_events.id) as count").
		Joins("left join block_lists on block_lists.id = block_domain_events.domain_id").
		Group("block_lists.url").
		Scan(&results).Error
	return results, err
}
