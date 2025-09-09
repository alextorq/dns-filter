package filter

import (
	"sync"

	"github.com/bits-and-blooms/bloom/v3"
)

var (
	globalFilter *bloom.BloomFilter
	once         sync.Once
)

func UpdateFilter(rows []string) {
	// создаём фильтр на 10000 элементов с вероятностью FP = 0.01
	globalFilter = bloom.NewWithEstimates(uint(len(rows)), 0.01)

	for _, item := range rows {
		globalFilter.Add([]byte(item))
	}
}

func GetFilter() *bloom.BloomFilter {
	once.Do(func() {
		globalFilter = bloom.NewWithEstimates(1000, 0.01)
	})
	return globalFilter
}
