package db

import (
	"fmt"
	"time"

	app_db "github.com/alextorq/dns-filter/db"
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

// ===== legacy shim =====
//
// These package-level functions are kept ONLY for callers that haven't been
// migrated to DI yet (filter, source). New code MUST construct *Repo via
// NewRepo(conn) at the composition root. All four functions delegate to a
// freshly-built Repo over the singleton connection, so the production path
// goes through exactly one implementation — no logic dupes between the
// package-level functions and Repo methods.
//
// Deprecated: each function disappears as its caller migrates to *Repo.

func legacyRepo() *Repo {
	return NewRepo(app_db.GetConnection())
}

// Deprecated: use Repo.GetAllActiveURLs.
func GetAllActiveFilters() ([]string, error) {
	return legacyRepo().GetAllActiveURLs()
}

// Deprecated: use Repo.IsActivelyBlocked.
func IsDomainActivelyBlocked(domain string) (bool, error) {
	return legacyRepo().IsActivelyBlocked(domain)
}

// Deprecated: use Repo.CreateDNSRecordsByDomains.
func CreateDNSRecordsByDomains(urls []string, source string) error {
	return legacyRepo().CreateDNSRecordsByDomains(urls, source)
}

// Deprecated: use Repo.ChangeRecordStatusBySource.
func ChangeRecordStatusBySource(source string, active bool) error {
	return legacyRepo().ChangeRecordStatusBySource(source, active)
}
