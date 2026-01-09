package filter

import (
	"fmt"
	"strconv"

	blacklists "github.com/alextorq/dns-filter/blocked-domain/db"
	filter_use_cases_change_filter_dns_records "github.com/alextorq/dns-filter/filter/business/use-cases/change-filter-dns-records"
	"github.com/alextorq/dns-filter/logger"
)

func ChangeFilterDnsRecords() bool {
	return filter_use_cases_change_filter_dns_records.ChangeFilterDnsRecords()
}

func UpdateFilterFromDb() error {
	list, err := blacklists.GetAllActiveFilters()
	if err != nil {
		return fmt.Errorf("ошибка получения данных из БД: %w", err)
	}
	l := logger.GetLogger()
	l.Info("Фильтр обновлён из БД, записей: " + strconv.Itoa(len(list)))
	f := GetFilter()
	f.UpdateFilter(list)
	return nil
}
