package editor

import (
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"

	texmath "muse/markdown/math"
)

type mathVisual struct {
	bmp      *ui.Bitmap
	fallback []mathFallbackGlyph
}

type mathFallbackGlyph struct {
	x, y   float32
	glyphs []ui.Glyph
	color  ui.Color
}

var (
	mathVisualCache = texmath.NewCache[*mathVisual](8<<20, func(_ string, visual *mathVisual) int64 {
		if visual == nil {
			return 0
		}
		var weight int64
		if visual.bmp != nil {
			w, h := visual.bmp.Size()
			weight += int64(w) * int64(h) * 4
		}
		weight += int64(len(visual.fallback)) * 64
		return weight
	})
	mathVisualMu sync.Mutex
)

func drawMathRaster(p *ui.Painter, dst ui.Rect, raster *texmath.Raster, dark bool, font string) {
	visual := mathVisualFor(raster, dark, font)
	if visual == nil || visual.bmp == nil || dst.W <= 0 || dst.H <= 0 {
		return
	}
	bw, bh := visual.bmp.Size()
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
		p.Image(visual.bmp, ui.Rect{X: x, Y: y, W: float32(bw) * natural, H: float32(bh) * natural}, ui.Contain)
		for _, glyph := range visual.fallback {
			p.Glyphs(glyph.glyphs, x+glyph.x*natural, y+glyph.y*natural, glyph.color)
		}
	})
}

func mathVisualFor(raster *texmath.Raster, dark bool, font string) *mathVisual {
	if raster == nil || raster.Image == nil {
		return nil
	}
	key := fmt.Sprintf("%p|%t|%s", raster, dark, font)
	if visual, ok := mathVisualCache.Load(key); ok {
		return visual
	}
	mathVisualMu.Lock()
	defer mathVisualMu.Unlock()
	if visual, ok := mathVisualCache.Load(key); ok {
		return visual
	}
	img := raster.Image
	if dark {
		img = texmath.InvertColors(img)
	}
	visual := &mathVisual{bmp: ui.NewBitmap(img)}
	for _, glyph := range raster.Fallback {
		shaped := ui.Shape(string(glyph.Rune), ui.Font{Family: font, Size: float32(raster.Size) * float32(glyph.Scale)})
		if len(shaped) == 0 {
			continue
		}
		ink := ui.Color{R: glyph.Color.R, G: glyph.Color.G, B: glyph.Color.B, A: glyph.Color.A}
		if dark {
			ink.R, ink.G, ink.B = 255-ink.R, 255-ink.G, 255-ink.B
		}
		visual.fallback = append(visual.fallback, mathFallbackGlyph{x: float32(glyph.X), y: float32(glyph.Y), glyphs: shaped, color: ink})
	}
	mathVisualCache.Store(key, visual)
	return visual
}
