package events

import (
	"time"

	"github.com/alextorq/dns-filter/db"
	"gorm.io/gorm"
)

type BlockDomainEvent struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deletedAt"`
	Domain    string         `gorm:"type:varchar(255);not null; index" json:"domain"`
}

func CreateBlockDomainEvent(domain string) error {
	conn := db.GetConnection()

	event := BlockDomainEvent{
		Domain: domain,
	}

	if err := conn.Create(&event).Error; err != nil {
		return err
	}
	return nil
}

func DeleteOlderThan(days int) error {
	conn := db.GetConnection()
	cutoff := time.Now().AddDate(0, 0, -days)
	if err := conn.Where("created_at < ?", cutoff).Delete(&BlockDomainEvent{}).Error; err != nil {
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
		Select("domain, COUNT(*) as count").
		Group("domain").
		Scan(&results).Error
	return results, err
}
