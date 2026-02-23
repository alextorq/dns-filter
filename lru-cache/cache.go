package lru_cache

import (
	"container/list"
	"sync"
)

type entry[T any] struct {
	key string
	val T
}

type LRUCache[T any] struct {
	capacity int // Максимальный размер
	items    map[string]*list.Element
	list     *list.List // двусвязный список (container/list)
	mu       sync.Mutex // Защита от гонок
}

func CreateCache[T any](capacity int) *LRUCache[T] {
	storage := LRUCache[T]{
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

func (c *LRUCache[T]) Add(key string, val T) AddReturn {
	c.mu.Lock()
	defer c.mu.Unlock()
	res := AddReturn{
		Evicted: false,
		Size:    0,
	}

	if el, ok := c.items[key]; ok {
		c.list.MoveToFront(el)
		el.Value.(*entry[T]).val = val // обновляем значение
		res.Size = c.list.Len()
		return res
	}
	first := c.list.PushFront(&entry[T]{
		key: key,
		val: val,
	})
	c.items[key] = first

	if c.list.Len() > c.capacity {
		last := c.list.Back()
		c.list.Remove(last)
		delete(c.items, last.Value.(*entry[T]).key)
		res.Evicted = true
	}

	res.Size = c.list.Len()
	return res
}

func (c *LRUCache[T]) Get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if item, found := c.items[key]; found {
		c.list.MoveToFront(item)
		return item.Value.(*entry[T]).val, true
	}
	var zero T
	return zero, false
}
