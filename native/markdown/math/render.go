package math

import (
	"image"
	"image/color"

	"github.com/luo-studio/go-tex/tex/layout"
	"github.com/luo-studio/go-tex/tex/mathstyle"
	texparser "github.com/luo-studio/go-tex/tex/parser"
	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"

	"muse/markdown/math/texcanvas"
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
	Fallback []FallbackGlyph
}

// Typeset parses, lays out, and rasterizes a formula without a UI backend.
// Fallback glyphs remain separate so a host renderer can use system fonts.
func Typeset(expr string, size float64, display bool) (*Raster, bool) {
	nodes, ok := parseTeX(expr)
	if !ok {
		if normalized := NormalizeTeX(expr); normalized != expr {
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
	raster := &Raster{Image: img, Baseline: float32(box.Height * px)}
	raster.Fallback = make([]FallbackGlyph, len(fallback))
	for i, glyph := range fallback {
		raster.Fallback[i] = FallbackGlyph{X: glyph.X, Y: glyph.Y, Scale: glyph.Scale, Rune: glyph.Rune, Color: glyph.Color}
	}
	return raster, true
}

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
