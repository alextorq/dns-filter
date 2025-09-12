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

func (c *LRUCache) Add(key string, val *dns.Msg) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.list.MoveToFront(el)
		el.Value.(*entry).val = val // обновляем значение
		return
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
	}
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

func (c *LRUCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.list.Len()
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
