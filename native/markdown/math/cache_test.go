package math

import "testing"

func TestCacheEvictsLeastRecentlyUsedEntries(t *testing.T) {
	cache := NewCache[string](10, nil)
	cache.Store("one", "1")
	cache.Store("two", "2")
	if _, ok := cache.Load("one"); !ok {
		t.Fatal("first entry should remain before the cache fills")
	}
	cache.Store("three", "3")
	if _, ok := cache.Load("two"); ok {
		t.Fatal("least recently used entry was not evicted")
	}
	if got, ok := cache.Load("one"); !ok || got != "1" {
		t.Fatalf("most recently used entry = %q, %v", got, ok)
	}
	if got, ok := cache.Load("three"); !ok || got != "3" {
		t.Fatalf("new entry = %q, %v", got, ok)
	}
	if entries, weight := cache.Stats(); entries != 2 || weight != 8 {
		t.Fatalf("cache stats = (%d entries, %d bytes), want (2, 8)", entries, weight)
	}
}

func TestCacheEvictsOversizedEntry(t *testing.T) {
	cache := NewCache[string](4, nil)
	cache.Store("oversized", "value")
	if _, ok := cache.Load("oversized"); ok {
		t.Fatal("oversized entry was retained")
	}
	if entries, weight := cache.Stats(); entries != 0 || weight != 0 {
		t.Fatalf("cache stats = (%d entries, %d bytes), want (0, 0)", entries, weight)
	}
}

func TestCacheAccountsForValueWeightAndReplacement(t *testing.T) {
	cache := NewCache[string](12, func(_ string, value string) int64 { return int64(len(value)) })
	cache.Store("key", "1234")
	if entries, weight := cache.Stats(); entries != 1 || weight != 7 {
		t.Fatalf("stats after store = (%d, %d), want (1, 7)", entries, weight)
	}
	cache.Store("key", "123456789012")
	if _, ok := cache.Load("key"); ok {
		t.Fatal("over-budget replacement was retained")
	}
	if entries, weight := cache.Stats(); entries != 0 || weight != 0 {
		t.Fatalf("stats after replacement eviction = (%d, %d), want (0, 0)", entries, weight)
	}
}
