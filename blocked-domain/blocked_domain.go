package blocked_domain

import (
	blocked_domain_use_cases_block_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/block-domain"
	blocked_domain_use_cases_clear_events "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/clear-events"
	blocked_domain_use_cases_create_domain "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/create-domain"
	blocked_domain_use_cases_seed "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/seed"
	blocked_domain_use_cases_update_dns_record "github.com/alextorq/dns-filter/blocked-domain/business/use-cases/update-dns-record"
	"github.com/alextorq/dns-filter/blocked-domain/db"
	dnsLib "github.com/miekg/dns"
)

// ===== Block Event Tracking (original blocked-domain functions) =====

func ClearOldEvent() {
	blocked_domain_use_cases_clear_events.ClearEvent()
}

func BlockDomain(w dnsLib.ResponseWriter, r *dnsLib.Msg) {
	blocked_domain_use_cases_block_domain.BlockDomain(w, r)
}

// ===== Blocklist Management (moved from dns-records) =====

// Sync loads blocklist from upstream source
func Sync() error {
	return blocked_domain_use_cases_seed.Sync()
}

// GetBlockListByID retrieves a block rule by ID
func GetBlockListByID(id uint) (*db.BlockList, error) {
	return db.GetBlockListByID(id)
}

// GetRecordsByFilter retrieves block rules with filtering and pagination
func GetRecordsByFilter(filter db.GetAllParams) (db.GetRecordsResult, error) {
	return db.GetRecordsByFilter(filter)
}

// GetAllActive retrieves all active block rules
func GetAllActive() ([]db.BlockList, error) {
	return db.GetAllActive()
}

// GetAllActiveFilters retrieves all active domain filters
func GetAllActiveFilters() ([]string, error) {
	return db.GetAllActiveFilters()
}

// DomainNotExist checks if a domain rule exists
func DomainNotExist(domain string) bool {
	return db.DomainNotExist(domain)
}

// GetDomainByName retrieves a block rule by domain name
func GetDomainByName(domain string) (db.BlockList, error) {
	return db.GetDomainByName(domain)
}

// GetAmountRecords returns the total count of block rules
func GetAmountRecords() int64 {
	return db.GetAmountRecords()
}

// CreateDNSRecordsByDomains creates multiple block rules from domain list
func CreateDNSRecordsByDomains(urls []string) error {
	return db.CreateDNSRecordsByDomains(urls)
}

// CreateDomain creates a new block rule
func CreateDomain(domain blocked_domain_use_cases_create_domain.RequestBody) error {
	return blocked_domain_use_cases_create_domain.CreateDomain(domain)
}

// UpdateDnsRecord updates a block rule's active status
func UpdateDnsRecord(update blocked_domain_use_cases_update_dns_record.UpdateBlockList) (*db.BlockList, error) {
	return blocked_domain_use_cases_update_dns_record.UpdateDnsRecord(update)
}

// LoadAll loads blocklist URLs from configured sources
func LoadAll() []string {
	return loadAll()
}
