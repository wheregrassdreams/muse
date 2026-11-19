package media

import (
	"github.com/egoist/mygo/ui"

	"muse/locales"
	texmath "muse/markdown/math"
)

// Viewport is an editor-local rectangle in logical pixels.
type Viewport struct {
	X, Y, W, H float32
}

// MathPreviewFrame is a detached view of an active formula preview.
type MathPreviewFrame struct {
	Viewport Viewport
	Anchor   Viewport
	Raster   *texmath.Raster
	Width    float32
	Height   float32
	FontSize float32
	Font     string
	Dark     bool
	Failed   bool
}

func DrawInlineMathPreview(ctx *ui.Context, frame MathPreviewFrame) {
	viewport := ui.Rect{X: frame.Viewport.X, Y: frame.Viewport.Y, W: frame.Viewport.W, H: frame.Viewport.H}
	anchor := ui.Rect{X: frame.Anchor.X, Y: frame.Anchor.Y, W: frame.Anchor.W, H: frame.Anchor.H}
	w, h := frame.Width+24, frame.Height+24
	card, ok := MathPreviewBounds(viewport, anchor, w, h)
	if !ok {
		return
	}
	ui.Overlay(ctx, func() {
		ui.Box(ctx).Absolute().Left(card.X).Top(card.Y).Width(card.W).Height(card.H).Draw(func(p *ui.Painter, r ui.Rect) {
			// The floating card needs an opaque surface so underlying text cannot
			// bleed through the math preview.
			p.Fill(r, p.Theme().Background, 8)
			p.Stroke(r, p.Theme().Border, 8, 1)
			p.Clip(r, 8, func() {
				if frame.Failed || frame.Raster == nil {
					p.RichText(r.X+12, r.Y+12, max(r.W-24, 1), ui.Span{Text: locales.EditorMathRenderError(), Size: frame.FontSize * 0.8, Color: p.Theme().TextMuted})
					return
				}
				dst := ui.Rect{X: r.X + 12, Y: r.Y + 12, W: max(r.W-24, 1), H: max(r.H-24, 1)}
				DrawMathRaster(p, dst, frame.Raster, frame.Dark, frame.Font)
			})
		})
	})
}

func MathPreviewBounds(view, anchor ui.Rect, w, h float32) (ui.Rect, bool) {
	const gap = float32(8)
	if view.W <= 2*gap || view.H <= 2*gap || anchor.Y+anchor.H <= view.Y || anchor.Y >= view.Y+view.H {
		return ui.Rect{}, false
	}
	w = min(w, view.W-2*gap)
	h = min(h, view.H-2*gap)
	y := anchor.Y + anchor.H + gap
	if y+h > view.Y+view.H-gap {
		y = anchor.Y - gap - h
	}
	y = max(view.Y+gap, min(y, view.Y+view.H-gap-h))
	x := max(view.X+gap, min(anchor.X, view.X+view.W-gap-w))
	return ui.Rect{X: x, Y: y, W: w, H: h}, true
}
