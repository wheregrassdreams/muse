package mdoc

import "testing"

func TestMathRenderCacheEvictsLeastRecentlyUsedEntries(t *testing.T) {
	cache := newMathRenderCache(10)
	cache.store("one", &mathRender{})
	cache.store("two", &mathRender{})
	if _, ok := cache.load("one"); !ok {
		t.Fatal("first entry should remain before the cache fills")
	}
	cache.store("three", &mathRender{})
	if _, ok := cache.load("two"); ok {
		t.Fatal("least recently used entry was not evicted")
	}
	if _, ok := cache.load("one"); !ok {
		t.Fatal("most recently used entry was evicted")
	}
	if _, ok := cache.load("three"); !ok {
		t.Fatal("new entry was not retained")
	}
	if entries, weight := cache.stats(); entries != 2 || weight != 8 {
		t.Fatalf("cache stats = (%d entries, %d bytes), want (2, 8)", entries, weight)
	}
}

func TestMathRenderCacheDoesNotRetainSingleOversizedFormula(t *testing.T) {
	cache := newMathRenderCache(4)
	cache.store("oversized", &mathRender{})
	if _, ok := cache.load("oversized"); ok {
		t.Fatal("oversized formula was retained")
	}
	if entries, weight := cache.stats(); entries != 0 || weight != 0 {
		t.Fatalf("cache stats after eviction = (%d entries, %d bytes), want (0, 0)", entries, weight)
	}
}
