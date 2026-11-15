package mdoc

import (
	"container/list"
	"image"
	"image/gif"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

// Keep a small working set of recently visited images so back-and-forth
// scrolling does not repeatedly decode them, without growing with every image
// the user has ever opened.
const maxImageAssetCacheBytes = 64 << 20
const maxCachedDataImageSourceBytes = 1 << 20

type imageAssetCacheKind uint8

const (
	imageAssetCacheFile imageAssetCacheKind = iota + 1
	imageAssetCacheRemote
	imageAssetCacheData
)

type imageAssetCacheKey struct {
	kind  imageAssetCacheKind
	value string
}

type cachedImageAsset struct {
	key       imageAssetCacheKey
	asset     *ImageAsset
	mod       time.Time
	size      int64
	weight    int64
	pinned    bool
	cacheable bool
	sourceKey dataImageSourceKey
}

type imageAssetCache struct {
	mu      sync.Mutex
	max     int64
	weight  int64
	entries map[imageAssetCacheKey]*list.Element
	sources map[dataImageSourceKey]*list.Element
	lru     list.List
}

var decodedImageAssets = newImageAssetCache(maxImageAssetCacheBytes)

func newImageAssetCache(max int64) *imageAssetCache {
	return &imageAssetCache{
		max:     max,
		entries: make(map[imageAssetCacheKey]*list.Element),
		sources: make(map[dataImageSourceKey]*list.Element),
	}
}

func (c *imageAssetCache) load(key imageAssetCacheKey) (*cachedImageAsset, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.lru.MoveToFront(elem)
	return elem.Value.(*cachedImageAsset), true
}

func (c *imageAssetCache) loadSource(key dataImageSourceKey) (*cachedImageAsset, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.sources[key]
	if !ok {
		return nil, false
	}
	c.lru.MoveToFront(elem)
	return elem.Value.(*cachedImageAsset), true
}

func (c *imageAssetCache) storeIfAbsent(entry *cachedImageAsset) (*cachedImageAsset, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[entry.key]; ok {
		c.lru.MoveToFront(elem)
		return elem.Value.(*cachedImageAsset), true
	}
	elem := c.lru.PushFront(entry)
	c.entries[entry.key] = elem
	if entry.sourceKey.data != nil {
		c.sources[entry.sourceKey] = elem
	}
	c.weight += entry.weight
	c.evictLocked()
	return entry, false
}

func (c *imageAssetCache) replace(entry *cachedImageAsset) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[entry.key]; ok {
		old := elem.Value.(*cachedImageAsset)
		c.weight -= old.weight
		if old.sourceKey.data != nil {
			delete(c.sources, old.sourceKey)
		}
		elem.Value = entry
		c.lru.MoveToFront(elem)
	} else {
		elem := c.lru.PushFront(entry)
		c.entries[entry.key] = elem
	}
	if entry.sourceKey.data != nil {
		c.sources[entry.sourceKey] = c.entries[entry.key]
	}
	c.weight += entry.weight
	c.evictLocked()
}

func (c *imageAssetCache) finish(key imageAssetCacheKey, asset *ImageAsset, weight int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[key]
	if !ok || elem.Value.(*cachedImageAsset).asset != asset {
		return false
	}
	entry := elem.Value.(*cachedImageAsset)
	if entry.sourceKey.data != nil {
		delete(c.sources, entry.sourceKey)
		entry.sourceKey = dataImageSourceKey{}
	}
	if !entry.cacheable {
		c.removeLocked(elem)
		return false
	}
	c.weight += weight - entry.weight
	entry.weight, entry.pinned = weight, false
	c.evictLocked()
	elem, ok = c.entries[key]
	return ok && elem.Value.(*cachedImageAsset).asset == asset
}

func (c *imageAssetCache) delete(key imageAssetCacheKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[key]; ok {
		c.removeLocked(elem)
	}
}

func (c *imageAssetCache) evictLocked() {
	for c.weight > c.max {
		var victim *list.Element
		for elem := c.lru.Back(); elem != nil; elem = elem.Prev() {
			if !elem.Value.(*cachedImageAsset).pinned {
				victim = elem
				break
			}
		}
		if victim == nil {
			return
		}
		c.removeLocked(victim)
	}
}

func (c *imageAssetCache) removeLocked(elem *list.Element) {
	entry := elem.Value.(*cachedImageAsset)
	delete(c.entries, entry.key)
	if entry.sourceKey.data != nil {
		delete(c.sources, entry.sourceKey)
	}
	c.weight -= entry.weight
	c.lru.Remove(elem)
}

func (c *imageAssetCache) stats() (entries int, weight int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries), c.weight
}

// ImageAssetCacheStats reports the number and estimated retained bytes of
// decoded image assets. It is intended for memory diagnostics; the byte
// estimate includes decoded pixel weight and cache keys, not GPU textures.
func ImageAssetCacheStats() (entries int, bytes int64) {
	return decodedImageAssets.stats()
}

// Estimate retained image memory conservatively. Static decoded images may
// also need a premultiplied RGBA bitmap; GIFs retain their source frames and a
// composited canvas while animating. Data URLs remain in the cache key, so
// account for that backing string too.
func imageAssetCacheWeight(key imageAssetCacheKey, frames []image.Image, direct *ui.Bitmap, animation *gif.GIF) int64 {
	weight := int64(len(key.value))
	if direct != nil {
		w, h := direct.Size()
		weight += int64(w) * int64(h) * 4
	}
	if animation != nil {
		var framePixels int64
		for _, frame := range animation.Image {
			if frame != nil {
				b := frame.Bounds()
				framePixels += int64(b.Dx()) * int64(b.Dy())
			}
		}
		w, h := animation.Config.Width, animation.Config.Height
		weight += framePixels + int64(w)*int64(h)*20
		return weight
	}
	for _, frame := range frames {
		if frame != nil {
			b := frame.Bounds()
			weight += int64(b.Dx()) * int64(b.Dy()) * 8
		}
	}
	return weight
}
