package dns_records

import (
	dns_records_use_cases_seed "github.com/alextorq/dns-filter/dns-records/business/use-cases/seed"
	dns_records_db "github.com/alextorq/dns-filter/dns-records/db"
)

// Facade functions for seed use-case
func Sync() error {
	return dns_records_use_cases_seed.Sync()
}

// Facade functions for database operations
func GetBlockListByID(id uint) (*dns_records_db.BlockList, error) {
	return dns_records_db.GetBlockListByID(id)
}

func GetRecordsByFilter(filter dns_records_db.GetAllParams) (dns_records_db.GetRecordsResult, error) {
	return dns_records_db.GetRecordsByFilter(filter)
}

func GetAllActive() ([]dns_records_db.BlockList, error) {
	return dns_records_db.GetAllActive()
}

func GetAllActiveFilters() ([]string, error) {
	return dns_records_db.GetAllActiveFilters()
}

func DomainNotExist(domain string) bool {
	return dns_records_db.DomainNotExist(domain)
}

func GetDomainByName(domain string) (dns_records_db.BlockList, error) {
	return dns_records_db.GetDomainByName(domain)
}

func GetAmountRecords() int64 {
	return dns_records_db.GetAmountRecords()
}

func CreateDNSRecordsByDomains(urls []string) error {
	return dns_records_db.CreateDNSRecordsByDomains(urls)
}

// Wrapper for load.go LoadAll
func LoadAll() []string {
	return loadAll()
}
