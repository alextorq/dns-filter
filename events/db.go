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
