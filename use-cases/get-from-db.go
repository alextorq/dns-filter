package use_cases

import (
	"fmt"
	"strconv"

	black_lists "github.com/alextorq/dns-filter/black-lists"
	"github.com/alextorq/dns-filter/filter"
	"github.com/alextorq/dns-filter/logger"

	"github.com/bits-and-blooms/bloom/v3"
)

func GetFromDb() (*bloom.BloomFilter, error) {
	list, err := black_lists.GetAllActiveFilters()
	if err != nil {
		return nil, fmt.Errorf("ошибка получения данных из БД: %w", err)
	}
	l := logger.GetLogger()
	l.Info("Фильтр обновлён из БД, записей: " + strconv.Itoa(len(list)))

	return filter.UpdateFilter(list), nil
}
