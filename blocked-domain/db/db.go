package db

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

type BlockList struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deletedAt"`
	Url       string         `gorm:"type:varchar(255);not null;uniqueIndex:idx_theme_host" json:"url"`
	Active    bool           `gorm:"default:true" json:"active"`
	Source    string         `gorm:"type:varchar(255)" json:"source"`
	// One-to-Many
	BlockedEvents []BlockDomainEvent `gorm:"foreignKey:DomainId" json:"blocked-events"`
}

func (r *BlockList) String() string {
	return fmt.Sprintf("BlockDomain[ID=%d, Domain=%s]", r.ID, r.Url)
}

// BlockDomainEvent tracks when a domain was blocked
type BlockDomainEvent struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	DomainId  uint
}

type GetAllParams struct {
	Limit  int
	Offset int
	Filter string
	Source string
}

type GetRecordsResult struct {
	Total int64       `json:"total"`
	List  []BlockList `json:"list"`
}

type DomainCount struct {
	Domain string `json:"domain"`
	Count  int64  `json:"count"`
}
