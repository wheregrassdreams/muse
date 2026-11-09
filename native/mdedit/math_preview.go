package mdedit

import "github.com/egoist/mygo/ui"

// mathPreviewBounds places a non-interactive card beside its source line.
// All coordinates are viewport coordinates; an offscreen anchor stays hidden.
func mathPreviewBounds(view, anchor ui.Rect, w, h float32) (ui.Rect, bool) {
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

// mathPreviewView paints only: no input handler, focus target or document space.
func (e *Editor) mathPreviewView(c *ui.Context, viewport ui.Rect) {
	if !e.focused {
		return
	}
	start, _, tex, ok := e.activeInlineMath()
	if !ok || e.composing || e.mathPreviewHidden {
		return
	}
	anchor, ok := e.caretRect(start)
	if !ok {
		return
	}
	anchor.X += viewport.X
	anchor.Y += viewport.Y - e.scroll.Y
	obj := e.mathObject(tex, e.lk.size, false)
	w, h := float32(240), float32(48)
	if obj != nil {
		w, h = obj.w+24, obj.h+obj.descent+24
	}
	card, ok := mathPreviewBounds(viewport, anchor, w, h)
	if !ok {
		return
	}
	ui.Overlay(c, func() {
		ui.Box(c).Absolute().Left(card.X).Top(card.Y).Width(card.W).Height(card.H).Draw(func(p *ui.Painter, r ui.Rect) {
			// A floating card needs an opaque surface: the dark code-panel
			// tint is translucent and would let quotation text show through.
			p.Fill(r, p.Theme().Background, 8)
			p.Stroke(r, e.lk.border, 8, 1)
			p.Clip(r, 8, func() {
				if obj == nil {
					p.RichText(r.X+12, r.Y+12, max(r.W-24, 1), ui.Span{Text: "公式尚未完成或无法渲染", Size: e.lk.size * 0.8, Color: e.lk.muted})
					return
				}
				dst := ui.Rect{X: r.X + 12, Y: r.Y + 12, W: max(r.W-24, 1), H: max(r.H-24, 1)}
				p.Image(obj.bmp, dst, ui.Contain)
			})
		})
	})
}
