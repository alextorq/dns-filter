package db

import "time"

type AllowDomainEvent struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Domain    string    `json:"domain" gorm:"uniqueIndex"`
	Active    bool      `json:"active"`
}
