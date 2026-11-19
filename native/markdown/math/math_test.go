package math

import "testing"

func TestTypesetCachedSharesRasterWithinBudget(t *testing.T) {
	const expr = `\int_{-3}^{5} x^2 \,dx`
	first, ok := TypesetCached(expr, 17, true)
	if !ok || first == nil || first.Image == nil {
		t.Fatal("formula did not produce a raster")
	}
	second, ok := TypesetCached(expr, 17, true)
	if !ok || second != first {
		t.Fatal("same formula request did not reuse its neutral raster")
	}
	if first.Size != 17 {
		t.Fatalf("raster size = %v, want 17", first.Size)
	}
	if entries, weight := RasterCacheStats(); entries == 0 || weight > rasterCacheBudget {
		t.Fatalf("raster cache stats = (%d entries, %d bytes), budget %d", entries, weight, rasterCacheBudget)
	}
}
