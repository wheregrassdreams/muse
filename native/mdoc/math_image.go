package mdoc

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"

	"git.sr.ht/~sbinet/gg"
	"github.com/egoist/mygo/ui"
	"github.com/go-latex/latex/drawtex"
	"github.com/go-latex/latex/font/ttf"
	"github.com/go-latex/latex/mtex"
	"github.com/go-latex/latex/tex"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// mathDPI is the rasterisation density; the bitmap is displayed at 72/DPI of
// its pixel size, so formulas stay sharp without being giant.
const mathDPI = 192.0

// typeset is a formula rasterized: its pixels, and its baseline in pixels
// from their top, which text around an inline formula lines up with.
type typeset struct {
	img      *image.RGBA
	baseline int
}

type mathRender struct {
	bmp      *ui.Bitmap
	baseline float32
}

var (
	mathCache sync.Map
	mathMu    sync.Mutex
)

// mathBitmap typesets a TeX expression with go-latex's engine (the same
// family of MathJax/KaTeX-style layout used by Typora), then inverts it for
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
	var img image.Image
	var err error
	if m, ok := parseMatrix(expr); ok {
		img, err = m.render(size)
		if err == nil {
			r.baseline = float32(img.Bounds().Dy())/2 + float32(size*mathDPI/72)*0.3
		}
	} else {
		var ts typeset
		ts, err = typesetMath(expr, size, display)
		img, r.baseline = ts.img, float32(ts.baseline)
	}
	if err == nil {
		if dark {
			img = invertImage(img)
		}
		r.bmp = ui.NewBitmap(img)
	}
	mathCache.Store(key, r)
	return r
}

// renderMathImage typesets expr at size (in points), as an image.
func renderMathImage(expr string, size float64) (image.Image, error) {
	ts, err := typesetMath(expr, size, false)
	if err != nil {
		return nil, err
	}
	return ts.img, nil
}

// typesetMath typesets expr at size (in points): with go-latex, or where it
// cannot lay out scripts, as on \int_0^1, with Muse's own composition of
// the base and its scripts.
func typesetMath(expr string, size float64, display bool) (typeset, error) {
	expr = normalizeTeX(expr)
	if display {
		// A block of math is displayed, not set in running text: fractions
		// are their full size and, where the base allows it, the limits of a
		// big operator go above and below it (see typesetScripts).
		expr = displayFracs(expr)
	}
	ts, err := typesetTeX(expr, size)
	if err == nil {
		return ts, nil
	}
	if strings.ContainsAny(expr, "^_") {
		if script, serr := typesetScripts(expr, size, display); serr == nil {
			return script, nil
		}
	}
	return typeset{}, err
}

// displayFracs makes the fractions of a displayed formula the size of the
// text around them: go-latex sets \frac in text style, as it is in a line of
// prose, and the display one is its \dfrac.
func displayFracs(expr string) string {
	expr = strings.ReplaceAll(expr, `\tfrac`, `\dfrac`)
	// \frac and \dfrac are the same length, so no \dfrac becomes \dfrac.
	return strings.ReplaceAll(expr, `\frac`, `\dfrac`)
}

// typesetTeX lays expr out with go-latex and rasterizes what it draws. The
// image is as large as the ink of every glyph and rule, rather than the
// box go-latex reports, which leaves out parts that hang below it.
func typesetTeX(expr string, size float64) (ts typeset, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("math panic: %v", r)
		}
	}()
	canvas := drawtex.New()
	box, err := mtex.Parse("$"+expr+"$", size, 72, ttf.New(canvas))
	if err != nil {
		return typeset{}, err
	}
	tree, ok := box.(tex.Tree)
	if !ok {
		return typeset{}, fmt.Errorf("math: %q laid out no tree", expr)
	}
	var sh tex.Ship
	sh.Call(0, 0, tree)

	// The ink, in points, with the box's own extent as the least of it.
	h := box.Height()
	minX, minY := 0.0, 0.0
	maxX, maxY := box.Width(), h+math.Max(box.Depth(), 0)
	for _, op := range canvas.Ops() {
		switch op := op.(type) {
		case drawtex.GlyphOp:
			m := op.Glyph.Metrics
			minX, maxX = math.Min(minX, op.X+m.XMin), math.Max(maxX, op.X+m.XMax)
			minY, maxY = math.Min(minY, op.Y-m.YMax), math.Max(maxY, op.Y-m.YMin)
		case drawtex.RectOp:
			minX, maxX = math.Min(minX, math.Min(op.X1, op.X2)), math.Max(maxX, math.Max(op.X1, op.X2))
			minY, maxY = math.Min(minY, math.Min(op.Y1, op.Y2)), math.Max(maxY, math.Max(op.Y1, op.Y2))
		}
	}
	const pad = 1.0
	s := mathDPI / 72
	w := int(math.Ceil((maxX - minX + 2*pad) * s))
	hpx := int(math.Ceil((maxY - minY + 2*pad) * s))
	if w <= 0 || hpx <= 0 {
		return typeset{}, fmt.Errorf("math: %q is empty", expr)
	}
	ctx := gg.NewContext(w, hpx)
	ctx.SetColor(color.Black)
	dx, dy := pad-minX, pad-minY
	for _, op := range canvas.Ops() {
		switch op := op.(type) {
		case drawtex.GlyphOp:
			face, err := opentype.NewFace(op.Glyph.Font, &opentype.FaceOptions{
				DPI:     mathDPI,
				Size:    op.Glyph.Size,
				Hinting: font.HintingNone,
			})
			if err != nil {
				return typeset{}, err
			}
			ctx.SetFontFace(face)
			ctx.DrawString(op.Glyph.Symbol, (op.X+dx)*s, (op.Y+dy)*s)
			face.Close()
		case drawtex.RectOp:
			x0, x1 := math.Min(op.X1, op.X2), math.Max(op.X1, op.X2)
			y0, y1 := math.Min(op.Y1, op.Y2), math.Max(op.Y1, op.Y2)
			ctx.DrawRectangle((x0+dx)*s, (y0+dy)*s, (x1-x0)*s, math.Max(y1-y0, 0.5)*s)
			ctx.Fill()
		}
	}
	img, ok := ctx.Image().(*image.RGBA)
	if !ok {
		return typeset{}, fmt.Errorf("math: unexpected image %T", ctx.Image())
	}
	return typeset{img: img, baseline: int(math.Round((h + dy) * s))}, nil
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
	fw := float32(w) * 72 / mathDPI * 1.4
	fh := float32(h) * 72 / mathDPI * 1.4
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
