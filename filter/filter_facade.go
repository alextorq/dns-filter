package filter

import (
	"fmt"
	"strconv"

	blacklists "github.com/alextorq/dns-filter/blocked-domain/db"
	filterusecaseschangefilterdnsrecords "github.com/alextorq/dns-filter/filter/business/use-cases/change-filter-dns-records"
	checkexist "github.com/alextorq/dns-filter/filter/business/use-cases/check-exist"
	filter2 "github.com/alextorq/dns-filter/filter/filter"
	"github.com/alextorq/dns-filter/logger"
)

func ChangeFilterDnsRecords() bool {
	return filterusecaseschangefilterdnsrecords.ChangeFilterDnsRecords()
}

func CheckExist(domain string) bool {
	return checkexist.CheckBlock(domain)
}

func UpdateFilterFromDb() error {
	list, err := blacklists.GetAllActiveFilters()
	if err != nil {
		return fmt.Errorf("ошибка получения данных из БД: %w", err)
	}
	l := logger.GetLogger()
	l.Info("Фильтр обновлён из БД, записей: " + strconv.Itoa(len(list)))
	f := filter2.GetFilter()
	f.UpdateFilter(list)
	return nil
}
