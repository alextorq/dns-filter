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

func Sync() error {
	return blocked_domain_use_cases_seed.Sync()
}

func GetRecordsByFilter(filter db.GetAllParams) (db.GetRecordsResult, error) {
	return db.GetRecordsByFilter(filter)
}

func GetAllActiveFilters() ([]string, error) {
	return db.GetAllActiveFilters()
}

func DomainNotExist(domain string) bool {
	return db.DomainNotExist(domain)
}

func CreateDNSRecordsByDomains(urls []string) error {
	return db.CreateDNSRecordsByDomains(urls)
}

func CreateDomain(domain blocked_domain_use_cases_create_domain.RequestBody) error {
	return blocked_domain_use_cases_create_domain.CreateDomain(domain)
}

func UpdateDnsRecord(update blocked_domain_use_cases_update_dns_record.UpdateBlockList) (*db.BlockList, error) {
	return blocked_domain_use_cases_update_dns_record.UpdateDnsRecord(update)
}
