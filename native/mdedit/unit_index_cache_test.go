package mdedit

import "testing"

func TestUnitIndexCachesRepeatedOffset(t *testing.T) {
	e := New("alpha\n\nbeta", Options{})
	units := e.units()
	want := e.findUnitIndex(units, 0)
	if got := e.unitIndex(units, 0); got != want {
		t.Fatalf("unitIndex() = %d, want %d", got, want)
	}
	if !e.unitIndexCache.valid {
		t.Fatal("unitIndex did not populate its cache")
	}
	// A deliberately different cached value proves the repeated lookup takes
	// the cached path, rather than merely returning the same scan result.
	e.unitIndexCache.index = 9876
	if got := e.unitIndex(units, 0); got != 9876 {
		t.Fatalf("repeated unitIndex() = %d, want cached result 9876", got)
	}
}

func TestUnitIndexCacheInvalidatesForFoldAndSourceChanges(t *testing.T) {
	e := New("alpha\n\nbeta", Options{})
	units := e.units()
	e.unitIndex(units, 0)
	e.unitIndexCache.index = 9876
	e.foldRevision++
	if got := e.unitIndex(units, 0); got == 9876 {
		t.Fatal("unitIndex reused a cache entry from an earlier fold revision")
	}

	e.unitIndexCache.index = 9876
	e.SetSource("gamma\n\nbeta")
	units = e.units()
	if got := e.unitIndex(units, 0); got == 9876 {
		t.Fatal("unitIndex reused a cache entry from an earlier document revision")
	}
}
