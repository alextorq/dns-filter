package cache

import (
	"container/list"
	"sync"

	"github.com/miekg/dns"
)

type entry struct {
	key string
	val *dns.Msg
}

type LRUCache struct {
	capacity int // максимальный размер
	items    map[string]*list.Element
	list     *list.List // двусвязный список (container/list)
	mu       sync.Mutex // защита от гонок
}

func CreateCache(capacity int) *LRUCache {
	storage := LRUCache{
		capacity: capacity,
		list:     list.New(),
		items:    make(map[string]*list.Element),
	}
	return &storage
}

type AddReturn struct {
	Evicted bool
	Size    int
}

func (c *LRUCache) Add(key string, val *dns.Msg) AddReturn {
	c.mu.Lock()
	defer c.mu.Unlock()
	res := AddReturn{
		Evicted: false,
		Size:    0,
	}

	if el, ok := c.items[key]; ok {
		c.list.MoveToFront(el)
		el.Value.(*entry).val = val // обновляем значение
		res.Size = c.list.Len()
		return res
	}
	first := c.list.PushFront(&entry{
		key: key,
		val: val,
	})
	c.items[key] = first

	if c.list.Len() > c.capacity {
		last := c.list.Back()
		c.list.Remove(last)
		delete(c.items, last.Value.(*entry).key)
		res.Evicted = true
	}

	res.Size = c.list.Len()
	return res
}

func (c *LRUCache) Get(key string) (*dns.Msg, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if item, found := c.items[key]; found {
		c.list.MoveToFront(item)
		return item.Value.(*entry).val, true
	}
	return nil, false
}

var (
	globalCache *LRUCache
	once        sync.Once
)

func GetCache() *LRUCache {
	once.Do(func() {
		globalCache = CreateCache(10000)
	})
	return globalCache
}
