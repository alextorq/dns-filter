package filter

import (
	"sync"

	"github.com/bits-and-blooms/bloom/v3"
)

type Filter struct {
	mu    sync.RWMutex
	Bloom *bloom.BloomFilter
}

var filter *Filter = nil
var once = sync.Once{}

func GetFilter() *Filter {
	once.Do(func() {
		if filter == nil {
			filter = &Filter{
				Bloom: bloom.NewWithEstimates(10, 0.001),
			}
		}
	})
	return filter
}

func (f *Filter) DomainExist(domain string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.Bloom.Test([]byte(domain))
}

func (f *Filter) UpdateFilter(rows []string) *Filter {
	filter := bloom.NewWithEstimates(uint(len(rows)), 0.001)
	for _, item := range rows {
		filter.Add([]byte(item))
	}

	f.mu.Lock()
	f.Bloom = filter
	f.mu.Unlock()
	return f
}
