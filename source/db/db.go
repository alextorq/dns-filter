package db

import (
	"time"

	"gorm.io/gorm"
)

type Source struct {
	ID        uint            `gorm:"primarykey" json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	DeletedAt gorm.DeletedAt  `gorm:"index" json:"deletedAt"`
	Name      BlockListSource `gorm:"type:varchar(255);not null;uniqueIndex:idx_sync_name" json:"name"`
	Active    bool            `json:"active"`
}

type GetAllParams struct {
	Limit  int
	Offset int
	Filter string
}
