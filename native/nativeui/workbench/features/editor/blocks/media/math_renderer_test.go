package media

import (
	"testing"

	texmath "muse/markdown/math"
)

func TestMathVisualBuildsFromNeutralRaster(t *testing.T) {
	raster, ok := texmath.TypesetCached(`\alpha + x`, 16, false)
	if !ok || raster == nil {
		t.Fatal("formula did not typeset")
	}
	light := MathVisualFor(raster, false, "")
	dark := MathVisualFor(raster, true, "")
	if light == nil || light.Bitmap() == nil || dark == nil || dark.Bitmap() == nil {
		t.Fatal("component math renderer did not create toolkit bitmaps")
	}
	if light == dark {
		t.Fatal("light and dark formula visuals shared a theme-specific bitmap")
	}
}
