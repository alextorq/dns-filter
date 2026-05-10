package db

import (
	"errors"
	"fmt"
	"time"

	"github.com/alextorq/dns-filter/db"
	"github.com/alextorq/dns-filter/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BlockList struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deletedAt"`
	Url       string         `gorm:"type:varchar(255);not null;uniqueIndex:idx_theme_host" json:"url"`
	Active    bool           `gorm:"default:true" json:"active"`
	Source    string         `gorm:"type:varchar(255)" json:"source"`
	// One-to-Many
	BlockedEvents []BlockDomainEvent `gorm:"foreignKey:DomainId" json:"blocked-events"`
}

func (r *BlockList) String() string {
	return fmt.Sprintf("BlockDomain[ID=%d, Domain=%s]", r.ID, r.Url)
}

func (r *BlockList) Update() error {
	conn := db.GetConnection()
	return conn.Save(r).Error
}

// BlockDomainEvent tracks when a domain was blocked
type BlockDomainEvent struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	DomainId  uint
}

// ===== BlockList Operations =====

func GetBlockListByID(id uint) (*BlockList, error) {
	conn := db.GetConnection()
	var blockList BlockList
	err := conn.Where("id = ?", id).First(&blockList).Error
	if err != nil {
		return nil, err
	}
	return &blockList, nil
}

type GetAllParams struct {
	Limit  int
	Offset int
	Filter string
	Source string
}

type GetRecordsResult struct {
	Total int64       `json:"total"`
	List  []BlockList `json:"list"`
}

func GetRecordsByFilter(filter GetAllParams) (GetRecordsResult, error) {
	conn := db.GetConnection()
	var lists []BlockList
	query := conn.Model(&BlockList{})
	var total int64

	// если нужно фильтровать по строке
	if filter.Filter != "" {
		query = query.Where("url LIKE ?", "%"+filter.Filter+"%")
	}
	// если нужно фильтровать по источнику
	if filter.Source != "" {
		query = query.Where("source = ?", filter.Source)
	}

	// сначала считаем количество
	query.Count(&total)

	err := query.
		Limit(filter.Limit).
		Offset(filter.Offset).
		Find(&lists).
		Error

	return GetRecordsResult{
		Total: total,
		List:  lists,
	}, err
}

func GetAllActiveFilters() ([]string, error) {
	conn := db.GetConnection()
	var urls []string
	err := conn.Model(&BlockList{}).
		Where("active = ?", true).
		Pluck("url", &urls).Error

	if err != nil {
		return nil, err
	}
	return urls, nil
}

func DomainNotExist(domain string) bool {
	conn := db.GetConnection()
	var blockList BlockList
	err := conn.Where("url = ?", domain).First(&blockList).Error
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func CreateDNSRecordsByDomains(urls []string, source string) error {
	if len(urls) == 0 {
		return nil
	}

	dedupedUrls := utils.OnlyUniqString(urls)
	entries := make([]BlockList, 0, len(dedupedUrls))
	for _, u := range dedupedUrls {
		entries = append(entries, BlockList{
			Url:    u,
			Active: true,
			Source: source,
		})
	}

	// Лимит SQLite — 32766 параметров на statement (с 3.32+).
	// BlockList пишет 6 колонок (id, created_at, updated_at, deleted_at, url, active, source) —
	// 5000 строк × 7 ≈ 35k. Берём 4000 с запасом.
	const batchSize = 4000

	// Вся пачка — одна транзакция: один fsync вместо одного на каждый батч.
	// OnConflict{DoNothing} опирается на uniqueIndex на Url и заменяет ручной pre-check.
	conn := db.GetConnection()
	return conn.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(entries, batchSize).Error
	})
}

func CreateDomain(domain string, source string) error {
	conn := db.GetConnection()
	newEntry := BlockList{
		Url:    domain,
		Active: true,
		Source: source,
	}
	return conn.Create(&newEntry).Error
}

func BatchCreateBlockDomainEvents(domains []string) error {
	conn := db.GetConnection()
	if len(domains) == 0 {
		return nil
	}

	// 1. Получаем ID всех уникальных доменов из батча.
	// Проекция id+url вместо BlockList целиком — index-only scan по idx_theme_host.
	uniqDomains := utils.OnlyUniqString(domains)
	type idUrl struct {
		ID  uint
		Url string
	}
	var rows []idUrl
	if err := conn.Model(&BlockList{}).
		Select("id", "url").
		Where("url IN ?", uniqDomains).
		Find(&rows).Error; err != nil {
		return err
	}

	// Создаем карту url -> id для быстрого поиска
	domainMap := make(map[string]uint, len(rows))
	for _, r := range rows {
		domainMap[r.Url] = r.ID
	}

	// 2. Формируем список событий, сохраняя исходное количество запросов
	var events []BlockDomainEvent
	for _, domain := range domains {
		if id, ok := domainMap[domain]; ok {
			events = append(events, BlockDomainEvent{DomainId: id})
		}
	}

	if len(events) == 0 {
		return nil
	}

	// 3. Пакетная вставка
	return conn.CreateInBatches(events, 100).Error
}

func DeleteOlderThan(days int) error {
	conn := db.GetConnection()
	cutoff := time.Now().AddDate(0, 0, -days)
	if err := conn.Unscoped().Where("created_at < ?", cutoff).Delete(&BlockDomainEvent{}).Error; err != nil {
		return err
	}
	return nil
}

func GetAmountRows() int64 {
	conn := db.GetConnection()
	var count int64
	conn.Model(&BlockDomainEvent{}).Count(&count)
	return count
}

type DomainCount struct {
	Domain string `json:"domain"`
	Count  int64  `json:"count"`
}

func GetRowsByDomains() ([]DomainCount, error) {
	conn := db.GetConnection()
	var results []DomainCount
	err := conn.Model(&BlockDomainEvent{}).
		Select("block_lists.url as domain, COUNT(block_domain_events.id) as count").
		Joins("left join block_lists on block_lists.id = block_domain_events.domain_id").
		Group("block_lists.url").
		Scan(&results).Error

	return results, err
}

func ChangeRecordStatusBySource(source string, active bool) error {
	conn := db.GetConnection()
	return conn.Model(&BlockList{}).Where("source = ?", source).
		Update("active", active).Error
}
