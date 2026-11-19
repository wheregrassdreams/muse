package math

import (
	"fmt"
	"image"
	"image/color"
	"sync"

	"github.com/luo-studio/go-tex/tex/layout"
	"github.com/luo-studio/go-tex/tex/mathstyle"
	texparser "github.com/luo-studio/go-tex/tex/parser"
	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
	editormath "github.com/wheregrassdreams/muse-editor/markdown/math"

	"muse/markdown/math/texcanvas"
)

const rasterCacheBudget = 8 << 20

var (
	rasterCache = NewCache[*Raster](rasterCacheBudget, func(_ string, raster *Raster) int64 {
		if raster == nil || raster.Image == nil {
			return 0
		}
		bounds := raster.Image.Bounds()
		return int64(bounds.Dx())*int64(bounds.Dy())*4 + int64(len(raster.Fallback))*32
	})
	rasterRenderMu sync.Mutex
)

// MathDPI is the raster density used for Markdown formulas: twice the DIP
// density used by the editor's text layout.
const MathDPI = 192.0

// MathScale converts formula raster pixels to DIPs.
const MathScale = 72.0 / MathDPI

// FallbackGlyph is a CJK rune left for the host text renderer while the
// remaining formula glyphs are rasterized with go-tex.
type FallbackGlyph struct {
	X, Y  float64
	Scale float64
	Rune  rune
	Color color.RGBA
}

// Raster is a backend-neutral typeset formula and its baseline in pixels.
type Raster struct {
	Image    image.Image
	Baseline float32
	Size     float64
	Fallback []FallbackGlyph
}

// Typeset parses, lays out, and rasterizes a formula without a UI backend.
// Fallback glyphs remain separate so a host renderer can use system fonts.
func Typeset(expr string, size float64, display bool) (*Raster, bool) {
	nodes, ok := parseTeX(expr)
	if !ok {
		if normalized := editormath.NormalizeTeX(expr); normalized != expr {
			nodes, ok = parseTeX(normalized)
		}
	}
	if !ok {
		return nil, false
	}
	opts := layout.DefaultOptions()
	if display {
		opts.Style = mathstyle.Display
	} else {
		opts.Style = mathstyle.Text
	}
	px := size * 192.0 / 72
	box := layout.Layout(nodes, opts)
	c, fallback := texcanvas.RenderWithFallback(layout.ToDisplayList(box), texcanvas.Options{FontSize: px, StrokeWidth: 1.5})
	img := rasterizer.Draw(c, canvas.DPMM(1), canvas.DefaultColorSpace)
	if img == nil {
		return nil, false
	}
	raster := &Raster{Image: img, Baseline: float32(box.Height * px), Size: size}
	raster.Fallback = make([]FallbackGlyph, len(fallback))
	for i, glyph := range fallback {
		raster.Fallback[i] = FallbackGlyph{X: glyph.X, Y: glyph.Y, Scale: glyph.Scale, Rune: glyph.Rune, Color: glyph.Color}
	}
	return raster, true
}

// TypesetCached returns a renderer-neutral raster result from a bounded LRU.
// Large expressions bypass the cache key retention cap but are still typeset
// normally for the caller that requested them.
func TypesetCached(expr string, size float64, display bool) (*Raster, bool) {
	if len(expr) > 64<<10 {
		return Typeset(expr, size, display)
	}
	key := fmt.Sprintf("%s|%.1f|%t", expr, size, display)
	if raster, ok := rasterCache.Load(key); ok {
		return raster, raster != nil
	}
	rasterRenderMu.Lock()
	defer rasterRenderMu.Unlock()
	if raster, ok := rasterCache.Load(key); ok {
		return raster, raster != nil
	}
	raster, ok := Typeset(expr, size, display)
	if !ok {
		return nil, false
	}
	rasterCache.Store(key, raster)
	return raster, true
}

// RasterCacheStats reports the neutral formula cache's retained entries and
// estimated bytes for diagnostics and boundary tests.
func RasterCacheStats() (entries int, weight int64) { return rasterCache.Stats() }

// InvertColors returns a formula raster with RGB channels inverted while
// preserving its alpha channel, for dark theme rendering.
func InvertColors(src image.Image) image.Image {
	if src == nil {
		return nil
	}
	b := src.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := src.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			dst.Set(x, y, color.RGBA{
				R: 255 - uint8(r>>8),
				G: 255 - uint8(g>>8),
				B: 255 - uint8(bl>>8),
				A: uint8(a >> 8),
			})
		}
	}
	return dst
}

func parseTeX(expr string) ([]texparser.Node, bool) {
	nodes, err := texparser.Parse(expr)
	if err != nil {
		return nil, false
	}
	return nodes, true
}
