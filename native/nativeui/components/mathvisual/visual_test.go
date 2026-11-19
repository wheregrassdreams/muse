package mathvisual

import (
	"testing"

	texmath "muse/markdown/math"
)

func TestPreviewAndEditorReuseTheSameThemeVisual(t *testing.T) {
	const expression = `\alpha + x`
	preview := Typeset(expression, 16, false, false, "")
	raster, ok := texmath.TypesetCached(expression, 16, false)
	if !ok || preview == nil {
		t.Fatal("formula did not typeset")
	}
	if editor := ForRaster(raster, false, ""); editor != preview {
		t.Fatal("preview and editor did not share the same cached visual")
	}
	if dark := ForRaster(raster, true, ""); dark == preview {
		t.Fatal("theme-specific bitmap was incorrectly shared")
	}
}
