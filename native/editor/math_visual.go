package editor

import (
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"
	texmath "muse/markdown/math"
)

// mathVisual is the editor's current toolkit adapter for the renderer-neutral
// typeset result. The feature package owns TeX parsing/layout/rasterization.
type mathVisual struct {
	bmp      *ui.Bitmap
	baseline float32
	fallback []mathFallbackGlyph
}

type mathFallbackGlyph struct {
	x, y   float32
	glyphs []ui.Glyph
	color  ui.Color
}

var (
	mathCache = texmath.NewCache[*mathVisual](8<<20, func(_ string, render *mathVisual) int64 {
		if render == nil {
			return 0
		}
		var weight int64
		if render.bmp != nil {
			w, h := render.bmp.Size()
			weight += int64(w) * int64(h) * 4
		}
		weight += int64(len(render.fallback)) * 64
		return weight
	})
	mathMu sync.Mutex
)

func renderMathVisual(expr string, size float64, dark, display bool, fontFamily string) *mathVisual {
	key := fmt.Sprintf("%s|%.1f|%v|%v|%s", expr, size, dark, display, fontFamily)
	if render, ok := mathCache.Load(key); ok {
		return render
	}
	mathMu.Lock()
	defer mathMu.Unlock()
	if render, ok := mathCache.Load(key); ok {
		return render
	}
	render := &mathVisual{}
	if raster, ok := texmath.Typeset(expr, size, display); ok {
		img := raster.Image
		if dark {
			img = texmath.InvertColors(img)
		}
		render.bmp = ui.NewBitmap(img)
		render.baseline = raster.Baseline
		for _, glyph := range raster.Fallback {
			shaped := ui.Shape(string(glyph.Rune), ui.Font{Family: fontFamily, Size: float32(size) * float32(glyph.Scale)})
			if len(shaped) == 0 {
				continue
			}
			ink := ui.Color{R: glyph.Color.R, G: glyph.Color.G, B: glyph.Color.B, A: glyph.Color.A}
			if dark {
				ink.R, ink.G, ink.B = 255-ink.R, 255-ink.G, 255-ink.B
			}
			render.fallback = append(render.fallback, mathFallbackGlyph{x: float32(glyph.X), y: float32(glyph.Y), glyphs: shaped, color: ink})
		}
	}
	mathCache.Store(key, render)
	return render
}

func (render *mathVisual) Bitmap() *ui.Bitmap {
	if render == nil {
		return nil
	}
	return render.bmp
}

func (render *mathVisual) Baseline() float32 {
	if render == nil {
		return 0
	}
	return render.baseline
}

func (render *mathVisual) Draw(p *ui.Painter, dst ui.Rect) {
	if render == nil || render.bmp == nil || dst.W <= 0 || dst.H <= 0 {
		return
	}
	bw, bh := render.bmp.Size()
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
		p.Image(render.bmp, ui.Rect{X: x, Y: y, W: float32(bw) * natural, H: float32(bh) * natural}, ui.Contain)
		for _, glyph := range render.fallback {
			p.Glyphs(glyph.glyphs, x+glyph.x*natural, y+glyph.y*natural, glyph.color)
		}
	})
}
