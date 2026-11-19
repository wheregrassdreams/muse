// Package mathvisual owns the shared toolkit presentation of backend-neutral
// TeX rasters for Markdown preview and editor media blocks.
package mathvisual

import (
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"

	texmath "muse/markdown/math"
)

type fallbackGlyph struct {
	x, y   float32
	glyphs []ui.Glyph
	color  ui.Color
}

// Visual is a cached toolkit bitmap plus shaped system-font fallback glyphs.
// It retains its raster key so pointer identity cannot be recycled while the
// corresponding presentation remains cached.
type Visual struct {
	raster   *texmath.Raster
	bitmap   *ui.Bitmap
	baseline float32
	fallback []fallbackGlyph
}

var (
	visuals = texmath.NewCache[*Visual](8<<20, func(_ string, visual *Visual) int64 {
		if visual == nil {
			return 0
		}
		var weight int64
		if visual.bitmap != nil {
			w, h := visual.bitmap.Size()
			weight += int64(w) * int64(h) * 4
		}
		if visual.raster != nil && visual.raster.Image != nil {
			bounds := visual.raster.Image.Bounds()
			weight += int64(bounds.Dx()) * int64(bounds.Dy()) * 4
		}
		return weight + int64(len(visual.fallback))*64
	})
	visualMu sync.Mutex
)

// Typeset renders one expression through the shared neutral raster cache and
// the shared MyGo visual cache.
func Typeset(expr string, size float64, dark, display bool, font string) *Visual {
	raster, ok := texmath.TypesetCached(expr, size, display)
	if !ok {
		return nil
	}
	return ForRaster(raster, dark, font)
}

// ForRaster returns the cached presentation for one neutral raster, theme,
// and fallback font.
func ForRaster(raster *texmath.Raster, dark bool, font string) *Visual {
	if raster == nil || raster.Image == nil {
		return nil
	}
	key := fmt.Sprintf("%p|%t|%s", raster, dark, font)
	if visual, ok := visuals.Load(key); ok {
		return visual
	}
	visualMu.Lock()
	defer visualMu.Unlock()
	if visual, ok := visuals.Load(key); ok {
		return visual
	}
	img := raster.Image
	if dark {
		img = texmath.InvertColors(img)
	}
	visual := &Visual{raster: raster, bitmap: ui.NewBitmap(img), baseline: raster.Baseline}
	for _, glyph := range raster.Fallback {
		shaped := ui.Shape(string(glyph.Rune), ui.Font{Family: font, Size: float32(raster.Size) * float32(glyph.Scale)})
		if len(shaped) == 0 {
			continue
		}
		ink := ui.Color{R: glyph.Color.R, G: glyph.Color.G, B: glyph.Color.B, A: glyph.Color.A}
		if dark {
			ink.R, ink.G, ink.B = 255-ink.R, 255-ink.G, 255-ink.B
		}
		visual.fallback = append(visual.fallback, fallbackGlyph{x: float32(glyph.X), y: float32(glyph.Y), glyphs: shaped, color: ink})
	}
	visuals.Store(key, visual)
	return visual
}

func (v *Visual) Bitmap() *ui.Bitmap {
	if v == nil {
		return nil
	}
	return v.bitmap
}

func (v *Visual) Baseline() float32 {
	if v == nil {
		return 0
	}
	return v.baseline
}

func (v *Visual) NaturalScale() float32 { return float32(texmath.MathScale) }

func (v *Visual) DrawFallback(p *ui.Painter, x, y, naturalScale float32) {
	if v == nil || p == nil {
		return
	}
	for _, glyph := range v.fallback {
		p.Glyphs(glyph.glyphs, x+glyph.x*naturalScale, y+glyph.y*naturalScale, glyph.color)
	}
}

// Draw centers the bitmap and its system-font fallback glyphs in dst.
func (v *Visual) Draw(p *ui.Painter, dst ui.Rect) {
	if v == nil || v.bitmap == nil || dst.W <= 0 || dst.H <= 0 {
		return
	}
	bw, bh := v.bitmap.Size()
	if bw <= 0 || bh <= 0 {
		return
	}
	scale := min(dst.W/float32(bw), dst.H/float32(bh))
	if scale <= 0 {
		return
	}
	w, h := float32(bw)*scale, float32(bh)*scale
	x, y := dst.X+(dst.W-w)/2, dst.Y+(dst.H-h)/2
	extra := scale / float32(texmath.MathScale)
	p.Transform(ui.Scaling(extra, extra), x, y, func() {
		natural := float32(texmath.MathScale)
		p.Image(v.bitmap, ui.Rect{X: x, Y: y, W: float32(bw) * natural, H: float32(bh) * natural}, ui.Contain)
		v.DrawFallback(p, x, y, natural)
	})
}
