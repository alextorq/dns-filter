package black_lists

import (
	"errors"
	"fmt"
	"time"

	"github.com/alextorq/dns-filter/db"
	"gorm.io/gorm"
)

type BlockList struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deletedAt"`
	Url       string         `gorm:"type:varchar(255);not null;uniqueIndex:idx_theme_host" json:"url"`
	Active    bool           `gorm:"default:true" json:"active"`
}

func (r *BlockList) String() string {
	return fmt.Sprintf("BlockDomain[ID=%d, Domain=%s]", r.ID, r.Url)
}

func (r *BlockList) Update() error {
	conn := db.GetConnection()
	return conn.Save(r).Error
}

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

func GetAllActive() ([]BlockList, error) {
	conn := db.GetConnection()
	var lists []BlockList
	err := conn.Find(&lists).Error
	if err != nil {
		return nil, err
	}
	return lists, nil
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

func GetAmountRecords() int64 {
	conn := db.GetConnection()
	var count int64
	conn.Model(&BlockList{}).Count(&count)
	return count
}

func CreateDNSRecordsByDomains(urls []string) error {
	conn := db.GetConnection()

	// 1. Найти уже существующие записи
	var existing []string
	if err := conn.Model(&BlockList{}).
		Where("url IN ?", urls).
		Pluck("url", &existing).Error; err != nil {
		return err
	}

	// 2. Сделать set для быстрого поиска
	existingSet := make(map[string]struct{}, len(existing))
	for _, e := range existing {
		existingSet[e] = struct{}{}
	}

	// 3. Собрать только новые записи
	var newEntries []BlockList
	for _, url := range urls {
		if _, found := existingSet[url]; !found {
			newEntries = append(newEntries, BlockList{
				Url:    url,
				Active: true,
			})
		}
	}

	// 4. Батч вставка (например, пачками по 1000)
	if len(newEntries) > 0 {
		if err := conn.CreateInBatches(newEntries, 1000).Error; err != nil {
			return err
		}
	}

	return nil
}

func CreateDomain(domain string) error {
	conn := db.GetConnection()
	newEntry := BlockList{
		Url:    domain,
		Active: true,
	}
	return conn.Create(&newEntry).Error
}
