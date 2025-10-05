package use_cases

import (
	"fmt"
	"strconv"

	blacklists "github.com/alextorq/dns-filter/dns-records"
	"github.com/alextorq/dns-filter/filter"
	"github.com/alextorq/dns-filter/logger"
)

func UpdateFilterFromDb() error {
	list, err := blacklists.GetAllActiveFilters()
	if err != nil {
		return fmt.Errorf("ошибка получения данных из БД: %w", err)
	}
	l := logger.GetLogger()
	l.Info("Фильтр обновлён из БД, записей: " + strconv.Itoa(len(list)))
	f := filter.GetFilter()
	f.UpdateFilter(list)
	return nil
}
