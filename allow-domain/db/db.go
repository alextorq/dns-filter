package db

import (
	"time"

	"github.com/alextorq/dns-filter/db"
	"gorm.io/gorm/clause"
)

type AllowDomainEvent struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Domain    string    `json:"domain" gorm:"uniqueIndex"`
	Active    bool      `json:"active"`
}

func CreateAllowDomainEvent(domain string) error {
	conn := db.GetConnection()

	event := AllowDomainEvent{
		Domain: domain,
		Active: true,
	}
	//Check for existing record to avoid duplicates
	var existingEvent AllowDomainEvent
	if conn.Where("domain = ?", domain).Limit(1).Find(&existingEvent).RowsAffected > 0 {
		// Запись существует, ничего не делаем
		return nil
	}

	return conn.Create(&event).Error
}

func CreateBatchDomains(domains []string) error {
	if len(domains) == 0 {
		return nil
	}
	conn := db.GetConnection()

	// Подготавливаем срез структур для вставки
	events := make([]AllowDomainEvent, 0, len(domains))
	for _, domain := range domains {
		events = append(events, AllowDomainEvent{
			Domain: domain,
			Active: true,
		})
	}
	// clause.OnConflict{DoNothing: true} говорит БД:
	// "Если запись с таким uniqueIndex уже есть, просто пропусти ее и не выдавай ошибку"
	return conn.Clauses(clause.OnConflict{
		DoNothing: true,
	}).Create(&events).Error
}

func DeleteOlderThan(days int) error {
	conn := db.GetConnection()
	cutoff := time.Now().AddDate(0, 0, -days)
	if err := conn.Unscoped().Where("created_at < ?", cutoff).Delete(&AllowDomainEvent{}).Error; err != nil {
		return err
	}
	return nil
}

func GetAllActiveFilters() ([]string, error) {
	conn := db.GetConnection()
	var domains []string
	err := conn.Model(&AllowDomainEvent{}).
		Where("active = ?", true).
		Pluck("domain", &domains).Error

	if err != nil {
		return nil, err
	}
	return domains, nil
}
