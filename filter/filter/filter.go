package filter

import (
	"sync"

	"github.com/bits-and-blooms/bloom/v3"
)

// expectedDomains is the design target documented in CLAUDE.md (10M items
// at 0.1% FP). It is also used as the floor in UpdateFilter so the bloom
// never collapses to zero capacity when the block list is small or empty —
// bloom.NewWithEstimates(0, _) produces a zero-bit filter that panics on Add.
const (
	expectedDomains = 10_000_000
	falsePositive   = 0.001
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
				Bloom: bloom.NewWithEstimates(expectedDomains, falsePositive),
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

func (f *Filter) UpdateFilter(rows []string) {
	n := max(uint(len(rows)), expectedDomains)
	filter := bloom.NewWithEstimates(n, falsePositive)
	for _, item := range rows {
		filter.Add([]byte(item))
	}

	f.mu.Lock()
	f.Bloom = filter
	f.mu.Unlock()
}
