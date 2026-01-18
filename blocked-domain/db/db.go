package db

import (
	"errors"
	"fmt"
	"time"

	"github.com/alextorq/dns-filter/db"
	"github.com/alextorq/dns-filter/utils"
	"gorm.io/gorm"
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

func GetDomainByName(domain string) (BlockList, error) {
	conn := db.GetConnection()
	var blockList BlockList
	err := conn.Where("url = ?", domain).First(&blockList).Error
	return blockList, err
}

func GetAmountRecords() int64 {
	conn := db.GetConnection()
	var count int64
	conn.Model(&BlockList{}).Count(&count)
	return count
}

func CreateDNSRecordsByDomains(urls []string, source string) error {
	conn := db.GetConnection()
	const chunkSize = 800 // безопасный размер для SQLite (лимит 999)

	dedupedUrls := utils.OnlyUniqString(urls)

	// --- 1. Находим уже существующие записи чанками ---
	var existing []string
	for i := 0; i < len(dedupedUrls); i += chunkSize {
		end := i + chunkSize
		if end > len(dedupedUrls) {
			end = len(dedupedUrls)
		}

		var part []string
		if err := conn.Model(&BlockList{}).
			Where("url IN ?", dedupedUrls[i:end]).
			Pluck("url", &part).Error; err != nil {
			return err
		}
		existing = append(existing, part...)
	}

	// --- 2. Делаем set из существующих ---
	existingSet := make(map[string]struct{}, len(existing))
	for _, e := range existing {
		existingSet[e] = struct{}{}
	}

	// --- 3. Собираем только новые записи ---
	var newEntries []BlockList
	for _, u := range dedupedUrls {
		if _, found := existingSet[u]; !found {
			newEntries = append(newEntries, BlockList{
				Url:    u,
				Active: true,
				Source: source,
			})
		}
	}

	// --- 4. Вставляем новые записи чанками ---
	for i := 0; i < len(newEntries); i += chunkSize {
		end := i + chunkSize
		if end > len(newEntries) {
			end = len(newEntries)
		}
		if err := conn.CreateInBatches(newEntries[i:end], chunkSize).Error; err != nil {
			return err
		}
	}

	return nil
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

func CreateBlockDomainEvent(domainId uint) error {
	conn := db.GetConnection()

	event := BlockDomainEvent{
		DomainId: domainId,
	}

	if err := conn.Create(&event).Error; err != nil {
		return err
	}
	return nil
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
	Domain string
	Count  int64
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
