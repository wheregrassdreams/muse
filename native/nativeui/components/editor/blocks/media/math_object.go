package media

import (
	"strings"

	texmath "muse/markdown/math"
)

const mathScale = texmath.MathScale

// MathObject is the immutable layout projection of one typeset formula.
type MathObject struct {
	Raster        *texmath.Raster
	Width, Height float32
	Descent       float32
	Block         bool
}

// TypesetInline creates a cached inline formula and its baseline metrics.
func TypesetInline(tex string, fontSize float32, display bool) (MathObject, bool) {
	tex = strings.TrimSpace(tex)
	if tex == "" {
		return MathObject{}, false
	}
	if display {
		fontSize *= 1.15
	}
	raster, ok := texmath.TypesetCached(tex, float64(fontSize), false)
	if !ok || raster == nil || raster.Image == nil {
		return MathObject{}, false
	}
	bounds := raster.Image.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	width, height := float32(w)*mathScale, float32(h)*mathScale
	ascent := min(raster.Baseline*mathScale, height)
	return MathObject{Raster: raster, Width: width + 2, Height: ascent, Descent: height - ascent}, true
}

// TypesetBlock creates a cached display formula constrained to the available
// width. A zero width leaves the intrinsic raster size unchanged.
func TypesetBlock(tex string, fontSize, width float32) (MathObject, bool) {
	tex = strings.TrimSpace(tex)
	if tex == "" {
		return MathObject{}, false
	}
	raster, ok := texmath.TypesetCached(tex, float64(fontSize*1.2), true)
	if !ok || raster == nil || raster.Image == nil {
		return MathObject{}, false
	}
	bounds := raster.Image.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	fw, fh := float32(w)*mathScale, float32(h)*mathScale
	if width > 0 && fw > width {
		fh = fh * width / fw
		fw = width
	}
	return MathObject{Raster: raster, Width: fw, Height: fh, Block: true}, true
}
