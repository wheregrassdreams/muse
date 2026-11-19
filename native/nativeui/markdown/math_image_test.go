package markdown

import "testing"

func TestNormalizedTeXWrappersRender(t *testing.T) {
	for _, expr := range []string{`\text{if } x>0`, `\left( x \right)`, `\mathbb{R}`, `\mathrm{abc}`} {
		if bmp := mathBitmap(expr, 18, false, false); bmp == nil {
			t.Fatalf("render %q: nothing drawn", expr)
		}
	}
}
