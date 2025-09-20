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

func CreateDNSRecordsByDomains(urls []string) error {
	conn := db.GetConnection()
	for _, url := range urls {
		var existing BlockList
		err := conn.Where("url = ?", url).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			newEntry := BlockList{
				Url:    url,
				Active: true,
			}
			if err := conn.Create(&newEntry).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}
