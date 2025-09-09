package filter

import (
	"github.com/bits-and-blooms/bloom/v3"
)

func UpdateFilter(rows []string) *bloom.BloomFilter {
	// создаём фильтр на 10000 элементов с вероятностью FP = 0.01
	filter := bloom.NewWithEstimates(uint(len(rows)), 0.01)

	for _, item := range rows {
		filter.Add([]byte(item))
	}

	return filter
}
