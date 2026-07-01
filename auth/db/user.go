package db

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	Login        string         `gorm:"type:varchar(255);not null;uniqueIndex" json:"login"`
	PasswordHash string         `gorm:"type:varchar(255);not null" json:"-"`
}
