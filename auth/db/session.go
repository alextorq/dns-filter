package db

import (
	"time"
)

type Session struct {
	Token     string    `gorm:"primaryKey;type:varchar(64)" json:"-"`
	UserID    uint      `gorm:"not null;index" json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `gorm:"index" json:"expires_at"`
}
