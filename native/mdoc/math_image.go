package mdoc

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/luo-studio/go-tex/tex/layout"
	"github.com/luo-studio/go-tex/tex/mathstyle"
	texparser "github.com/luo-studio/go-tex/tex/parser"
	"github.com/luo-studio/go-tex/tex/render"
)

// MathDPI is the density formulas are rasterised at: twice a DIP. A bitmap
// is drawn at MathScale of its pixel size, so a formula of the same size as
// the text around it is the same height as that text.
const MathDPI = 192.0

// MathScale turns the pixels of a formula into DIPs.
const MathScale = 72.0 / MathDPI

// mathRender is a formula rasterized: its bitmap, and its baseline in pixels
// from the top, which text around an inline formula lines up with.
type mathRender struct {
	bmp      *ui.Bitmap
	baseline float32
}

var (
	mathCache sync.Map
	mathMu    sync.Mutex
)

// mathBitmap typesets a TeX expression and rasterises it, inverting it for
// the dark theme. A cache keeps a formula from being re-rasterised each frame.
func mathBitmap(expr string, size float64, dark bool, display bool) *ui.Bitmap {
	return mathRendered(expr, size, dark, display).bmp
}

// mathRendered is mathBitmap with the formula's baseline.
func mathRendered(expr string, size float64, dark bool, display bool) *mathRender {
	key := fmt.Sprintf("%s|%.1f|%v|%v", expr, size, dark, display)
	if v, ok := mathCache.Load(key); ok {
		return v.(*mathRender)
	}
	mathMu.Lock()
	defer mathMu.Unlock()
	if v, ok := mathCache.Load(key); ok {
		return v.(*mathRender)
	}
	r := &mathRender{}
	if img, baseline, ok := typesetMath(expr, size, display); ok {
		if dark {
			img = invertImage(img)
		}
		r.bmp = ui.NewBitmap(img)
		r.baseline = baseline
	}
	mathCache.Store(key, r)
	return r
}

// typesetMath lays a formula out with go-tex — a port of KaTeX, with KaTeX's
// own font metrics and glyphs — and rasterises it at MathDPI. display asks
// for the style of a formula on a line of its own: fractions at full size and
// the limits of a big operator above and below it.
//
// The expression is read as it is; a formula KaTeX's parser will not take is
// read again with the older normalisation, which is our own approximation
// (it drops \left and \right, for one) and is only a fallback.
func typesetMath(expr string, size float64, display bool) (image.Image, float32, bool) {
	nodes, ok := parseTeX(expr)
	if !ok {
		if n := normalizeTeX(expr); n != expr {
			nodes, ok = parseTeX(n)
		}
	}
	if !ok {
		return nil, 0, false
	}
	opts := layout.DefaultOptions()
	if display {
		opts.Style = mathstyle.Display
	} else {
		opts.Style = mathstyle.Text
	}
	// The style has to be in the layout, not in the parser: it changes the
	// sizes and the placement of the whole box tree.
	box := layout.Layout(nodes, opts)
	var buf bytes.Buffer
	err := render.PNGTo(&buf, layout.ToDisplayList(box),
		render.WithFontSize(size),
		render.WithPadding(0),
		render.WithScale(MathDPI/72),
		render.WithStrokeWidth(1))
	if err != nil {
		return nil, 0, false
	}
	img, err := png.Decode(&buf)
	if err != nil {
		return nil, 0, false
	}
	return img, float32(box.Height * size * MathDPI / 72), true
}

// parseTeX parses a formula, reporting whether the parser took it.
func parseTeX(expr string) ([]texparser.Node, bool) {
	nodes, err := texparser.Parse(expr)
	if err != nil {
		return nil, false
	}
	return nodes, true
}

func invertImage(src image.Image) image.Image {
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

// drawMathBlock shows the typeset formula, capped to the content width. The
// Unicode fallback in renderBlock is used if the expression cannot be
// typeset.
func drawMathBlock(c *ui.Context, bmp *ui.Bitmap, s rstyle) {
	w, h := bmp.Size()
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
		ui.Image(c, bmp).Size(fw, fh)
	})
}

// RenderMath exposes the cached math raster to the native editor.
func RenderMath(expr string, size float64, dark bool) *ui.Bitmap {
	return mathBitmap(expr, size, dark, true)
}

// RenderMathBaseline is RenderMath with the formula's baseline, in pixels
// of the bitmap from its top, which the text around an inline formula lines
// up with.
func RenderMathBaseline(expr string, size float64, dark bool) (*ui.Bitmap, float32) {
	r := mathRendered(expr, size, dark, false)
	return r.bmp, r.baseline
}
