package markdown

import (
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"

	texmath "muse/markdown/math"
)

// MathDPI is the density formulas are rasterised at: twice a DIP. A bitmap
// is drawn at MathScale of its pixel size, so a formula of the same size as
// the text around it is the same height as that text.
const MathDPI = 192.0

// MathScale turns the pixels of a formula into DIPs.
const MathScale = 72.0 / MathDPI

// MathVisual is a formula rendered with go-tex's math glyphs and the host
// text renderer's system-font fallback for CJK characters.
type MathVisual struct {
	bmp      *ui.Bitmap
	baseline float32
	fallback []nativeFallbackGlyph
}

type nativeFallbackGlyph struct {
	x, y   float32 // canvas pixels; y is the baseline
	glyphs []ui.Glyph
	color  ui.Color
}

var (
	mathCache = texmath.NewCache[*MathVisual](8<<20, func(_ string, render *MathVisual) int64 {
		if render == nil {
			return 0
		}
		var weight int64
		if render.bmp != nil {
			w, h := render.bmp.Size()
			weight += int64(w) * int64(h) * 4
		}
		// Include retained system-shaped glyph handles in the bounded math
		// cache estimate; these remain much smaller than formula bitmaps.
		weight += int64(len(render.fallback)) * 64
		return weight
	})
	mathMu sync.Mutex
)

// mathBitmap typesets a TeX expression and rasterises it, inverting it for
// the dark theme. A cache keeps a formula from being re-rasterised each frame.
func mathBitmap(expr string, size float64, dark bool, display bool) *ui.Bitmap {
	return mathRendered(expr, size, dark, display, "").bmp
}

// mathRendered is mathBitmap with baseline and native CJK glyphs.
func mathRendered(expr string, size float64, dark bool, display bool, fontFamily string) *MathVisual {
	key := fmt.Sprintf("%s|%.1f|%v|%v|%s", expr, size, dark, display, fontFamily)
	if r, ok := mathCache.Load(key); ok {
		return r
	}
	mathMu.Lock()
	defer mathMu.Unlock()
	if r, ok := mathCache.Load(key); ok {
		return r
	}
	r := &MathVisual{}
	if raster, ok := texmath.Typeset(expr, size, display); ok {
		img := raster.Image
		if dark {
			img = texmath.InvertColors(img)
		}
		r.bmp = ui.NewBitmap(img)
		r.baseline = raster.Baseline
		for _, g := range raster.Fallback {
			glyphs := ui.Shape(string(g.Rune), ui.Font{Family: fontFamily, Size: float32(size) * float32(g.Scale)})
			if len(glyphs) == 0 {
				continue
			}
			c := ui.Color{R: g.Color.R, G: g.Color.G, B: g.Color.B, A: g.Color.A}
			if dark {
				c.R, c.G, c.B = 255-c.R, 255-c.G, 255-c.B
			}
			r.fallback = append(r.fallback, nativeFallbackGlyph{x: float32(g.X), y: float32(g.Y), glyphs: glyphs, color: c})
		}
	}
	mathCache.Store(key, r)
	return r
}

// Bitmap returns the rasterized math-font portion of the formula.
func (r *MathVisual) Bitmap() *ui.Bitmap {
	if r == nil {
		return nil
	}
	return r.bmp
}

// Baseline returns the formula baseline in bitmap pixels from its top.
func (r *MathVisual) Baseline() float32 {
	if r == nil {
		return 0
	}
	return r.baseline
}

// Draw paints the math bitmap and its system-font CJK glyphs into dst. The
// glyph layouts are shaped once and reused with the same proportional scale
// as the bitmap, keeping equation positioning stable without a CJK font copy.
func (r *MathVisual) Draw(p *ui.Painter, dst ui.Rect) {
	if r == nil || r.bmp == nil || dst.W <= 0 || dst.H <= 0 {
		return
	}
	bw, bh := r.bmp.Size()
	if bw <= 0 || bh <= 0 {
		return
	}
	scale := min(dst.W/float32(bw), dst.H/float32(bh))
	if scale <= 0 {
		return
	}
	w, h := float32(bw)*scale, float32(bh)*scale
	x, y := dst.X+(dst.W-w)/2, dst.Y+(dst.H-h)/2
	extra := scale / float32(MathScale)
	p.Transform(ui.Scaling(extra, extra), x, y, func() {
		natural := float32(MathScale)
		p.Image(r.bmp, ui.Rect{X: x, Y: y, W: float32(bw) * natural, H: float32(bh) * natural}, ui.Contain)
		for _, g := range r.fallback {
			p.Glyphs(g.glyphs, x+g.x*natural, y+g.y*natural, g.color)
		}
	})
}

// drawMathBlock shows the typeset formula, capped to the content width. The
// Unicode fallback in renderBlock is used if the expression cannot be
// typeset.
func drawMathBlock(c *ui.Context, visual *MathVisual, s rstyle) {
	if visual == nil || visual.bmp == nil {
		return
	}
	w, h := visual.bmp.Size()
	fw := float32(w) * 72 / MathDPI * 1.4
	fh := float32(h) * 72 / MathDPI * 1.4
	maxW := float32(800)
	if s.o.ContentWidth > 0 {
		maxW = s.o.ContentWidth
	}
	if fw > maxW {
		fh = fh * maxW / fw
		fw = maxW
	}
	ui.Row(c).FillWidth().Justify(ui.Center).Padding(8, 0).Children(func() {
		ui.Box(c).Size(fw, fh).Draw(func(p *ui.Painter, r ui.Rect) { visual.Draw(p, r) })
	})
}

// RenderMath exposes only the math-font raster. UI code that needs CJK
// fallback must use RenderMathVisual and draw the result with MathVisual.Draw.
func RenderMath(expr string, size float64, dark bool) *ui.Bitmap {
	return mathBitmap(expr, size, dark, true)
}

// RenderMathVisual typesets a formula for native UI drawing. fontFamily is
// the same family used for surrounding text; empty selects the system default.
func RenderMathVisual(expr string, size float64, dark bool, display bool, fontFamily string) *MathVisual {
	return mathRendered(expr, size, dark, display, fontFamily)
}

// RenderMathBaseline is the math-font raster and its baseline in pixels.
// UI code that needs CJK fallback must use RenderMathVisual instead.
func RenderMathBaseline(expr string, size float64, dark bool) (*ui.Bitmap, float32) {
	r := mathRendered(expr, size, dark, false, "")
	return r.bmp, r.baseline
}
