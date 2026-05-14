package blocked_domain

import (
	blocked_domain_use_cases_block_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/block-domain"
	blocked_domain_use_cases_clear_events "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/clear-events"
	blocked_domain_use_cases_create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	blocked_domain_use_cases_update_dns_record "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/update-dns-record"
	"github.com/alextorq/dns-filter/blocked-domain/db"
	app_db "github.com/alextorq/dns-filter/db"
	"github.com/alextorq/dns-filter/logger"
)

// DEPRECATED: this whole file is a transitional shim during the singleton →
// DI migration (architecture cleanup п.3). External callers should switch to
// constructing *db.Repo at the composition root and passing it explicitly;
// once they do, this file is removed entirely (see T7).
//
// Until then, every facade function wraps a fresh Repo built from the global
// connection so the behavior is identical to the pre-refactor code, while the
// internal use-cases already speak DI.

// repo is the shim's per-call Repo factory. Allocates a thin wrapper around
// the singleton *gorm.DB (no connection leak — GORM pools internally), but
// hides DI behind a package-level call. Do NOT copy this pattern outside
// this file: new code constructs *db.Repo at the composition root and passes
// it explicitly.
func repo() *db.Repo {
	return db.NewRepo(app_db.GetConnection())
}

// ===== Block Event Tracking (original blocked-domain functions) =====

func ClearOldEvent() {
	blocked_domain_use_cases_clear_events.ClearEvent(repo())
}

func CreateBlockDomainEventStore(capacity int) *blocked_domain_use_cases_block_domain.BlockDomainEventStore {
	return blocked_domain_use_cases_block_domain.NewBlockDomainEventStore(repo(), logger.GetLogger(), capacity)
}

// ===== Blocklist Management (moved from dns-records) =====

func GetRecordsByFilter(filter db.GetAllParams) (db.GetRecordsResult, error) {
	return repo().GetRecordsByFilter(filter)
}

func GetAllActiveFilters() ([]string, error) {
	return repo().GetAllActiveURLs()
}

func DomainNotExist(domain string) bool {
	return repo().DomainNotExist(domain)
}

func IsDomainActivelyBlocked(domain string) (bool, error) {
	return repo().IsActivelyBlocked(domain)
}

func CreateDomain(domain blocked_domain_use_cases_create_domain.RequestBody) error {
	return blocked_domain_use_cases_create_domain.CreateDomain(
		blocked_domain_use_cases_create_domain.Deps{Repo: repo(), Log: logger.GetLogger()},
		domain,
	)
}

func UpdateDnsRecord(update blocked_domain_use_cases_update_dns_record.UpdateBlockList) (*db.BlockList, error) {
	return blocked_domain_use_cases_update_dns_record.UpdateDnsRecord(
		blocked_domain_use_cases_update_dns_record.Deps{Repo: repo(), Log: logger.GetLogger()},
		update,
	)
}
