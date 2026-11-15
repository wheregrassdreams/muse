package markdown

import (
	"image"
	"image/gif"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/markdown/images"
)

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
type dataImageSourceKey struct {
	data *byte
	len  int
}

type cachedImageAsset struct {
	key               imageAssetCacheKey
	asset             *ImageAsset
	mod               time.Time
	size, weight      int64
	pinned, cacheable bool
	sourceKey         dataImageSourceKey
}

// imageAssetCache is a compatibility adapter for the nativeui asset handle;
// LRU ownership, byte accounting, pinning and eviction live in markdown/images.
type imageAssetCache struct{ core *images.AssetCache }

var decodedImageAssets = newImageAssetCache(maxImageAssetCacheBytes)

func newImageAssetCache(max int64) *imageAssetCache {
	return &imageAssetCache{core: images.NewAssetCache(max)}
}
func toCoreKey(k imageAssetCacheKey) images.AssetKey {
	return images.AssetKey{Kind: images.AssetKeyKind(k.kind), Value: k.value}
}
func toCoreSource(k dataImageSourceKey) images.SourceIdentity {
	return images.SourceIdentity{Data: k.data, Length: k.len}
}
func toCoreEntry(e *cachedImageAsset) *images.AssetCacheEntry {
	return &images.AssetCacheEntry{Key: toCoreKey(e.key), Value: e.asset, ModTime: e.mod.UnixNano(), Size: e.size, Weight: e.weight, Pinned: e.pinned, Cacheable: e.cacheable, Source: toCoreSource(e.sourceKey)}
}
func fromCoreEntry(e *images.AssetCacheEntry) *cachedImageAsset {
	if e == nil {
		return nil
	}
	return &cachedImageAsset{key: imageAssetCacheKey{kind: imageAssetCacheKind(e.Key.Kind), value: e.Key.Value}, asset: e.Value.(*ImageAsset), mod: time.Unix(0, e.ModTime), size: e.Size, weight: e.Weight, pinned: e.Pinned, cacheable: e.Cacheable, sourceKey: dataImageSourceKey{data: e.Source.Data, len: e.Source.Length}}
}
func (c *imageAssetCache) load(key imageAssetCacheKey) (*cachedImageAsset, bool) {
	e, ok := c.core.Load(toCoreKey(key))
	if !ok {
		return nil, false
	}
	return fromCoreEntry(e), true
}
func (c *imageAssetCache) loadSource(key dataImageSourceKey) (*cachedImageAsset, bool) {
	e, ok := c.core.LoadSource(toCoreSource(key))
	if !ok {
		return nil, false
	}
	return fromCoreEntry(e), true
}
func (c *imageAssetCache) storeIfAbsent(entry *cachedImageAsset) (*cachedImageAsset, bool) {
	e, loaded := c.core.StoreIfAbsent(toCoreEntry(entry))
	return fromCoreEntry(e), loaded
}
func (c *imageAssetCache) replace(entry *cachedImageAsset) { c.core.Replace(toCoreEntry(entry)) }
func (c *imageAssetCache) finish(key imageAssetCacheKey, asset *ImageAsset, weight int64) bool {
	return c.core.Finish(toCoreKey(key), asset, weight)
}
func (c *imageAssetCache) delete(key imageAssetCacheKey) { c.core.Delete(toCoreKey(key)) }
func (c *imageAssetCache) stats() (int, int64)           { return c.core.Stats() }

// ImageAssetCacheStats reports feature cache entries and estimated retained
// bytes, excluding renderer/GPU allocations.
func ImageAssetCacheStats() (entries int, bytes int64) { return decodedImageAssets.stats() }

func imageAssetCacheWeight(key imageAssetCacheKey, frames []image.Image, direct *ui.Bitmap, animation *gif.GIF) int64 {
	var bitmapSize image.Point
	if direct != nil {
		w, h := direct.Size()
		bitmapSize = image.Pt(w, h)
	}
	return images.EstimateRetainedBytes(len(key.value), frames, bitmapSize, animation)
}
