package images

import (
	"image"
	"testing"
)

func TestAssetCacheEvictsLRUAndEnforcesCompletedWeight(t *testing.T) {
	c := NewAssetCache(10)
	a := &Asset{}
	b := &Asset{}
	c.StoreIfAbsent(&AssetCacheEntry{Key: AssetKey{Kind: AssetFile, Value: "a"}, Asset: a, Weight: 6})
	c.StoreIfAbsent(&AssetCacheEntry{Key: AssetKey{Kind: AssetFile, Value: "b"}, Asset: b, Weight: 6})
	if _, ok := c.Load(AssetKey{Kind: AssetFile, Value: "a"}); ok {
		t.Fatal("least-recently-used entry survived eviction")
	}
	if got, ok := c.Load(AssetKey{Kind: AssetFile, Value: "b"}); !ok || got.Asset != b {
		t.Fatal("most-recently-used entry was evicted")
	}
	if n, w := c.Stats(); n != 1 || w != 6 {
		t.Fatalf("cache stats=(%d,%d), want (1,6)", n, w)
	}

	key := AssetKey{Kind: AssetData, Value: "oversized"}
	large := &Asset{}
	c.StoreIfAbsent(&AssetCacheEntry{Key: key, Asset: large, Weight: 1, Pinned: true, Cacheable: true})
	c.Finish(key, large, 11)
	if _, ok := c.Load(key); ok {
		t.Fatal("completed oversized asset stayed cached")
	}
}

func TestEstimateRetainedBytesIncludesDecodedAndAnimationBudget(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 10, 5))
	if got := EstimateRetainedBytes(10, []image.Image{frame}, image.Point{}, nil); got != 10+10*5*8 {
		t.Fatalf("static weight=%d", got)
	}
}
