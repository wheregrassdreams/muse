package mdoc

import (
	"container/list"
	"sync"
)

type mathCacheEntry struct {
	key    string
	render *MathVisual
	weight int64
}

type mathRenderCache struct {
	mu      sync.Mutex
	max     int64
	weight  int64
	entries map[string]*list.Element
	lru     list.List
}

func newMathRenderCache(max int64) *mathRenderCache {
	return &mathRenderCache{max: max, entries: make(map[string]*list.Element)}
}

func (c *mathRenderCache) load(key string) (*MathVisual, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.lru.MoveToFront(elem)
	return elem.Value.(*mathCacheEntry).render, true
}

func (c *mathRenderCache) store(key string, render *MathVisual) {
	weight := int64(len(key))
	if render != nil && render.bmp != nil {
		w, h := render.bmp.Size()
		weight += int64(w) * int64(h) * 4
		// Include retained system-shaped glyph handles in the bounded math
		// cache's estimate. They are small compared with the formula bitmap.
		weight += int64(len(render.fallback)) * 64
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[key]; ok {
		entry := elem.Value.(*mathCacheEntry)
		c.weight -= entry.weight
		entry.render, entry.weight = render, weight
		c.weight += weight
		c.lru.MoveToFront(elem)
	} else {
		entry := &mathCacheEntry{key: key, render: render, weight: weight}
		c.entries[key] = c.lru.PushFront(entry)
		c.weight += weight
	}
	for c.weight > c.max {
		oldest := c.lru.Back()
		if oldest == nil {
			break
		}
		entry := oldest.Value.(*mathCacheEntry)
		delete(c.entries, entry.key)
		c.weight -= entry.weight
		c.lru.Remove(oldest)
	}
}

func (c *mathRenderCache) stats() (entries int, weight int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries), c.weight
}
