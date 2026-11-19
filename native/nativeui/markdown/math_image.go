package markdown

import (
	"github.com/egoist/mygo/ui"
	texmath "muse/markdown/math"
	"muse/nativeui/components/mathvisual"
)

// MathVisual is a shared cached formula bitmap and its system-font fallback.
type MathVisual = mathvisual.Visual

func mathBitmap(expr string, size float64, dark bool, display bool) *ui.Bitmap {
	visual := mathvisual.Typeset(expr, size, dark, display, "")
	if visual == nil {
		return nil
	}
	return visual.Bitmap()
}

func mathRendered(expr string, size float64, dark bool, display bool, fontFamily string) *MathVisual {
	return mathvisual.Typeset(expr, size, dark, display, fontFamily)
}

func drawMathBlock(c *ui.Context, visual *MathVisual, s rstyle) {
	if visual == nil || visual.Bitmap() == nil {
		return
	}
	w, h := visual.Bitmap().Size()
	fw := float32(w) * 72 / texmath.MathDPI * 1.4
	fh := float32(h) * 72 / texmath.MathDPI * 1.4
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

// RenderMath exposes the math-font bitmap portion of a formula.
func RenderMath(expr string, size float64, dark bool) *ui.Bitmap {
	return mathBitmap(expr, size, dark, true)
}

// RenderMathVisual typesets a formula with the host text renderer's CJK font.
func RenderMathVisual(expr string, size float64, dark bool, display bool, fontFamily string) *MathVisual {
	return mathRendered(expr, size, dark, display, fontFamily)
}

// RenderMathBaseline returns the formula bitmap and its baseline in pixels.
func RenderMathBaseline(expr string, size float64, dark bool) (*ui.Bitmap, float32) {
	visual := mathRendered(expr, size, dark, false, "")
	if visual == nil {
		return nil, 0
	}
	return visual.Bitmap(), visual.Baseline()
}
