package images

import (
	"container/list"
	"image"
	"image/gif"
	"sync"
)

// AssetKeyKind identifies a normalized image source for decoded asset reuse.
type AssetKeyKind uint8

const (
	AssetFile AssetKeyKind = iota + 1
	AssetRemote
	AssetData
)

// AssetKey identifies a local, remote, or embedded image source.
type AssetKey struct {
	Kind  AssetKeyKind
	Value string
}

// SourceIdentity coalesces concurrent data URL requests without cloning large
// Markdown substrings into cache keys.
type SourceIdentity struct {
	Data   *byte
	Length int
}

// AssetCacheEntry is an opaque decoded asset retained by the feature cache.
// Value is intentionally renderer-neutral to avoid a dependency on MyGo.
type AssetCacheEntry struct {
	Key       AssetKey
	Asset     *Asset
	ModTime   int64
	Size      int64
	Weight    int64
	Pinned    bool
	Cacheable bool
	Source    SourceIdentity
}

// AssetCache is a bounded LRU shared by image renderers. In-flight entries may
// be pinned; completion updates their estimated retained pixel weight.
type AssetCache struct {
	mu          sync.Mutex
	max, weight int64
	entries     map[AssetKey]*list.Element
	sources     map[SourceIdentity]*list.Element
	lru         list.List
}

func NewAssetCache(maxBytes int64) *AssetCache {
	return &AssetCache{max: maxBytes, entries: make(map[AssetKey]*list.Element), sources: make(map[SourceIdentity]*list.Element)}
}

func (c *AssetCache) Load(key AssetKey) (*AssetCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.lru.MoveToFront(e)
	return e.Value.(*AssetCacheEntry), true
}
func (c *AssetCache) LoadSource(key SourceIdentity) (*AssetCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.sources[key]
	if !ok {
		return nil, false
	}
	c.lru.MoveToFront(e)
	return e.Value.(*AssetCacheEntry), true
}
func (c *AssetCache) StoreIfAbsent(entry *AssetCacheEntry) (*AssetCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[entry.Key]; ok {
		c.lru.MoveToFront(e)
		return e.Value.(*AssetCacheEntry), true
	}
	e := c.lru.PushFront(entry)
	c.entries[entry.Key] = e
	if entry.Source.Data != nil {
		c.sources[entry.Source] = e
	}
	c.weight += entry.Weight
	c.evictLocked()
	return entry, false
}
func (c *AssetCache) Replace(entry *AssetCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[entry.Key]; ok {
		old := e.Value.(*AssetCacheEntry)
		c.weight -= old.Weight
		if old.Source.Data != nil {
			delete(c.sources, old.Source)
		}
		e.Value = entry
		c.lru.MoveToFront(e)
	} else {
		e := c.lru.PushFront(entry)
		c.entries[entry.Key] = e
	}
	if entry.Source.Data != nil {
		c.sources[entry.Source] = c.entries[entry.Key]
	}
	c.weight += entry.Weight
	c.evictLocked()
}
func (c *AssetCache) Finish(key AssetKey, asset *Asset, weight int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return false
	}
	entry := e.Value.(*AssetCacheEntry)
	if entry.Asset != asset {
		return false
	}
	if entry.Source.Data != nil {
		delete(c.sources, entry.Source)
		entry.Source = SourceIdentity{}
	}
	if !entry.Cacheable {
		c.removeLocked(e)
		return false
	}
	c.weight += weight - entry.Weight
	entry.Weight, entry.Pinned = weight, false
	c.evictLocked()
	e, ok = c.entries[key]
	return ok && e.Value.(*AssetCacheEntry).Asset == asset
}
func (c *AssetCache) Delete(key AssetKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		c.removeLocked(e)
	}
}
func (c *AssetCache) Stats() (int, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries), c.weight
}
func (c *AssetCache) evictLocked() {
	for c.weight > c.max {
		var victim *list.Element
		for e := c.lru.Back(); e != nil; e = e.Prev() {
			if !e.Value.(*AssetCacheEntry).Pinned {
				victim = e
				break
			}
		}
		if victim == nil {
			return
		}
		c.removeLocked(victim)
	}
}
func (c *AssetCache) removeLocked(e *list.Element) {
	entry := e.Value.(*AssetCacheEntry)
	delete(c.entries, entry.Key)
	if entry.Source.Data != nil {
		delete(c.sources, entry.Source)
	}
	c.weight -= entry.Weight
	c.lru.Remove(e)
}

// EstimateRetainedBytes estimates a decoded source's retained pixel weight.
// It accounts for renderer bitmap copies and the indexed frames plus four
// compositing canvases used by the lazy GIF animator.
func EstimateRetainedBytes(encodedKeyBytes int, frames []image.Image, bitmapSize image.Point, animation *gif.GIF) int64 {
	weight := int64(encodedKeyBytes)
	if bitmapSize.X > 0 && bitmapSize.Y > 0 {
		weight += int64(bitmapSize.X) * int64(bitmapSize.Y) * 4
	}
	if animation != nil {
		var indexed int64
		for _, f := range animation.Image {
			if f != nil {
				b := f.Bounds()
				indexed += int64(b.Dx()) * int64(b.Dy())
			}
		}
		return weight + indexed + int64(animation.Config.Width)*int64(animation.Config.Height)*20
	}
	for _, f := range frames {
		if f != nil {
			b := f.Bounds()
			weight += int64(b.Dx()) * int64(b.Dy()) * 8
		}
	}
	return weight
}
