package blocked_domain

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
	DomainId  uint           `gorm:"index"`
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

func ResetBlockDomainEventsTable() error {
	conn := db.GetConnection()
	// Удаляем таблицу
	if err := conn.Migrator().DropTable(&BlockDomainEvent{}); err != nil {
		return err
	}

	// Создаём заново по модели
	if err := conn.AutoMigrate(&BlockDomainEvent{}); err != nil {
		return err
	}

	return nil
}

func init() {
	ResetBlockDomainEventsTable()
}

func GetRowsByDomains() ([]DomainCount, error) {
	conn := db.GetConnection()
	var results []DomainCount
	err := conn.Model(&BlockDomainEvent{}).
		Select("block_lists.url as url, COUNT(block_domain_events.id) as count").
		Joins("left join block_lists on block_lists.id = block_domain_events.domain_id").
		Group("block_lists.url").
		Scan(&results).Error

	return results, err
}
