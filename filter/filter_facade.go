package filter

import (
	"fmt"
	"strconv"

	blacklists "github.com/alextorq/dns-filter/blocked-domain/db"
	filterusecaseschangefilterdnsrecords "github.com/alextorq/dns-filter/filter/business/use-cases/change-filter-dns-records"
	checkexist "github.com/alextorq/dns-filter/filter/business/use-cases/check-exist"
	"github.com/alextorq/dns-filter/filter/cache"
	filter2 "github.com/alextorq/dns-filter/filter/filter"
	"github.com/alextorq/dns-filter/logger"
)

func ChangeFilterDnsRecords() bool {
	return filterusecaseschangefilterdnsrecords.ChangeFilterDnsRecords()
}

func CheckExist(domain string) bool {
	return checkexist.CheckBlock(domain)
}

// UpdateFilterFromDb rebuilds the bloom filter from the DB and discards the
// LRU verdict cache. Both must move together: a stale cache after a mutation
// would otherwise serve the old verdict for ~1500 lookups.
func UpdateFilterFromDb() error {
	list, err := blacklists.GetAllActiveFilters()
	if err != nil {
		return fmt.Errorf("ошибка получения данных из БД: %w", err)
	}
	l := logger.GetLogger()
	l.Info("Фильтр обновлён из БД, записей: " + strconv.Itoa(len(list)))
	f := filter2.GetFilter()
	f.UpdateFilter(list)
	cache.GetCache().Clear()
	return nil
}
