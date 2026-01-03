package filter

import (
	filter_use_cases_change_filter_dns_records "github.com/alextorq/dns-filter/filter/business/use-cases/change-filter-dns-records"
)

func ChangeFilterDnsRecords() bool {
	return filter_use_cases_change_filter_dns_records.ChangeFilterDnsRecords()
}
