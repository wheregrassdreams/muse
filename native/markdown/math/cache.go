package math

import (
	"container/list"
	"sync"
)

type cacheEntry[T any] struct {
	key    string
	value  T
	weight int64
}

// Cache is a byte-budgeted LRU for expensive math render results.
type Cache[T any] struct {
	mu      sync.Mutex
	max     int64
	weight  int64
	weigh   func(string, T) int64
	entries map[string]*list.Element
	lru     list.List
}

// NewCache creates an LRU with a retained-weight budget and value estimator.
func NewCache[T any](max int64, weigh func(key string, value T) int64) *Cache[T] {
	return &Cache[T]{max: max, weigh: weigh, entries: make(map[string]*list.Element)}
}

// Load returns a cached value and marks it as recently used.
func (c *Cache[T]) Load(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[key]
	if !ok {
		var zero T
		return zero, false
	}
	c.lru.MoveToFront(elem)
	return elem.Value.(*cacheEntry[T]).value, true
}

// Store adds or replaces an entry and evicts least-recently-used entries
// until the retained estimate is within the configured budget.
func (c *Cache[T]) Store(key string, value T) {
	weight := int64(len(key))
	if c.weigh != nil {
		weight += c.weigh(key, value)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[key]; ok {
		entry := elem.Value.(*cacheEntry[T])
		c.weight -= entry.weight
		entry.value, entry.weight = value, weight
		c.weight += weight
		c.lru.MoveToFront(elem)
	} else {
		entry := &cacheEntry[T]{key: key, value: value, weight: weight}
		c.entries[key] = c.lru.PushFront(entry)
		c.weight += weight
	}
	for c.weight > c.max {
		oldest := c.lru.Back()
		if oldest == nil {
			break
		}
		entry := oldest.Value.(*cacheEntry[T])
		delete(c.entries, entry.key)
		c.weight -= entry.weight
		c.lru.Remove(oldest)
	}
}

// Stats reports the retained entry count and estimated weight.
func (c *Cache[T]) Stats() (entries int, weight int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries), c.weight
}
