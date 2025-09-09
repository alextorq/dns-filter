package use_cases

import (
	"dns-filter/db"
	"dns-filter/filter"
	"dns-filter/logger"
	"fmt"
	"strconv"

	"github.com/bits-and-blooms/bloom/v3"
)

func GetFromDb() (*bloom.BloomFilter, error) {
	list, err := db.GetAllRowsWhereActive()
	if err != nil {
		return nil, fmt.Errorf("ошибка получения данных из БД: %w", err)
	}
	l := logger.GetLogger()
	l.Info("Фильтр обновлён из БД, записей: " + strconv.Itoa(len(list)))

	return filter.UpdateFilter(list), nil
}
