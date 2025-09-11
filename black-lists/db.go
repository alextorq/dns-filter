package black_lists

import (
	"errors"
	"fmt"

	"github.com/alextorq/dns-filter/db"
	"gorm.io/gorm"
)

type BlockList struct {
	gorm.Model
	Url    string `gorm:"type:varchar(255);not null;uniqueIndex:idx_theme_host"`
	Active bool   `gorm:"default:true"`
}

func (u BlockList) String() string {
	return fmt.Sprintf("BlockDomain[ID=%d, Domain=%s]", u.ID, u.Url)
}

func GetAllActiveFilters() ([]string, error) {
	conn := db.GetConnection()
	var lists []BlockList
	err := conn.Where("active = ?", true).Find(&lists).Error
	if err != nil {
		return nil, err
	}
	var urls []string
	for _, list := range lists {
		urls = append(urls, list.Url)
	}
	_ = urls
	return urls, nil
}

func GetBlockListByDomain(domain string) (BlockList, error) {
	conn := db.GetConnection()
	var blockList BlockList
	err := conn.Where("url = ?", domain).First(&blockList).Error
	return blockList, err
}

func CreateFilter(urls []string) error {
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
