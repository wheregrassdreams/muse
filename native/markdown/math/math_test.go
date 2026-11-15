package math

import "testing"

func TestTeXToUnicodeFallback(t *testing.T) {
	cases := map[string]string{
		`\alpha + \beta`: "α + β",
		`E = mc^2`:       "E = mc²",
		`x_i`:            "xᵢ",
		`\frac{a}{b}`:    "a/b",
		`\sqrt{x}`:       "√(x)",
		`a \le b \ge c`:  "a ≤ b ≥ c",
	}
	for in, want := range cases {
		if got := TeXToUnicode(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}

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
