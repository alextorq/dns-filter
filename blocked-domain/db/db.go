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
	// Reasons holds the auto-block signal codes (#95); read paths must
	// Preload("Reasons") to populate it. See CreateDomainWithReasons.
	Reasons []BlockListReason `gorm:"foreignKey:BlockListID;constraint:OnDelete:CASCADE" json:"reasons,omitempty"`
}

func (r *BlockList) String() string {
	return fmt.Sprintf("BlockDomain[ID=%d, Domain=%s]", r.ID, r.Url)
}

// BlockListReason is one signal that caused a domain to land on the blocklist.
// Mirrors suggest-to-block's SuggestBlockReason but hangs off block_lists, so
// the reason an AutoBlocked domain was promoted survives in the DB without
// depending on application logs (#95). Code is a stable signal code from
// collect (subdomain_of_blocked, similar_to_blocked, …); MatchValue optionally
// carries the related blocked domain for comparison signals.
type BlockListReason struct {
	ID          uint   `gorm:"primarykey" json:"id"`
	BlockListID uint   `gorm:"index;not null" json:"-"`
	Code        string `gorm:"index;not null" json:"code"`
	MatchValue  string `json:"match,omitempty"`
}

// BlockDomainEvent tracks when a domain was blocked. DomainId is indexed: it is
// the column the high-volume events table is filtered/joined on — the stats
// aggregation (GetEventsByDomain joins block_lists on domain_id) and the stale
// prune (DeleteDNSRecordsBySourceNotIn deletes WHERE domain_id IN (...)). Without
// the index both degrade to full table scans as the events table grows.
type BlockDomainEvent struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	DomainId  uint      `gorm:"index"`
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
