package db

import (
	"time"

	"github.com/alextorq/dns-filter/db"
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
	if err := conn.Where("domain = ?", domain).First(&existingEvent).Error; err == nil {
		// Record already exists, no need to create a new one
		return nil
	}

	return conn.Create(&event).Error
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
