package markdown

import (
	"image"
	"testing"
	"time"
	"unsafe"
)

func TestImageAssetCacheEvictsLeastRecentlyUsedWithinWeightLimit(t *testing.T) {
	cache := newImageAssetCache(10)
	first := &cachedImageAsset{
		key:    imageAssetCacheKey{kind: imageAssetCacheFile, value: "first"},
		asset:  &ImageAsset{},
		weight: 6,
	}
	second := &cachedImageAsset{
		key:    imageAssetCacheKey{kind: imageAssetCacheFile, value: "second"},
		asset:  &ImageAsset{},
		weight: 6,
	}
	cache.storeIfAbsent(first)
	cache.storeIfAbsent(second)
	if _, ok := cache.load(first.key); ok {
		t.Fatal("least recently used entry was not evicted")
	}
	if _, ok := cache.load(second.key); !ok {
		t.Fatal("most recently used entry was evicted")
	}
	if entries, weight := cache.stats(); entries != 1 || weight != 6 {
		t.Fatalf("cache stats = (%d entries, %d bytes), want (1, 6)", entries, weight)
	}
}

func TestImageAssetCacheOversizedAssetIsNotRetained(t *testing.T) {
	cache := newImageAssetCache(10)
	asset := &ImageAsset{}
	key := imageAssetCacheKey{kind: imageAssetCacheData, value: "large"}
	sourceKey := dataImageSourceKey{data: unsafe.StringData(key.value), len: len(key.value)}
	entry := &cachedImageAsset{key: key, asset: asset, sourceKey: sourceKey, weight: 1, pinned: true}
	cache.storeIfAbsent(entry)
	cache.finish(key, asset, 11)
	if _, ok := cache.load(key); ok {
		t.Fatal("oversized asset was retained")
	}
	if _, ok := cache.loadSource(sourceKey); ok {
		t.Fatal("evicted source identity still points into the cache")
	}
	if entries, weight := cache.stats(); entries != 0 || weight != 0 {
		t.Fatalf("cache stats after eviction = (%d entries, %d bytes), want (0, 0)", entries, weight)
	}
}

func TestImageAssetCacheDropsNonCacheableSourceAfterDecode(t *testing.T) {
	cache := newImageAssetCache(10)
	key := imageAssetCacheKey{kind: imageAssetCacheData, value: "large-source"}
	sourceKey := dataImageSourceKey{data: unsafe.StringData(key.value), len: len(key.value)}
	asset := &ImageAsset{}
	cache.storeIfAbsent(&cachedImageAsset{
		key: key, asset: asset, weight: int64(len(key.value)), pinned: true,
		cacheable: false, sourceKey: sourceKey,
	})
	if _, ok := cache.loadSource(sourceKey); !ok {
		t.Fatal("in-flight large source was not deduplicated")
	}
	if retained := cache.finish(key, asset, int64(len(key.value))); retained {
		t.Fatal("non-cacheable source was retained after decode")
	}
	if _, ok := cache.loadSource(sourceKey); ok {
		t.Fatal("completed source identity remained cached")
	}
	if _, ok := cache.load(key); ok {
		t.Fatal("completed large source remained cached")
	}
}

func TestStaticImageAssetDropsDecodedSourceAfterBitmapCreation(t *testing.T) {
	asset := &ImageAsset{
		frames: []image.Image{image.NewYCbCr(image.Rect(0, 0, 32, 24), image.YCbCrSubsampleRatio420)},
		loaded: true,
	}
	if next, changed := asset.Advance(time.Now()); next != 0 || !changed {
		t.Fatalf("static image Advance = (%v, %v), want (0, true)", next, changed)
	}
	if asset.Bitmap() == nil {
		t.Fatal("bitmap was not created")
	}
	if len(asset.frames) != 0 {
		t.Fatalf("static decoded source retains %d frames, want none", len(asset.frames))
	}
}
