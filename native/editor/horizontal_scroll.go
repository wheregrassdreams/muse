package editor

import (
	"github.com/egoist/mygo/ui"
	"muse/ui/scrollbar"
)

// Horizontal scroll belongs to the displayed block, never to source offsets
// or the document's vertical scroll. Cells share their table's viewport.
type blockHorizontalScroll struct {
	x         float32
	shown     bool
	indicator *scrollbar.Indicator
}

type blockHorizontalDrag struct {
	leaf        *leaf
	start, from float32
}

func tableViewportWidth(t *leaf) float32 {
	if t == nil || t.geom == nil {
		return 0
	}
	return min(t.w, t.geom.w)
}

func tableContentX(t *leaf) float32 { return t.x - t.horizontal.x }

func blockScrollOffset(u *leaf) float32 {
	if u == nil || u.table != nil {
		return 0 // cell.x already includes the table's scroll
	}
	return u.horizontal.x
}

func unitTextX(u *leaf, v *leafView) float32 {
	return u.x + v.tx - blockScrollOffset(u)
}

func blockViewport(l *leaf) ui.Rect {
	w := l.w
	if l.kind == kTable {
		w = tableViewportWidth(l)
	}
	return ui.Rect{X: l.x, Y: l.y, W: w, H: l.h}
}

func blockContentWidth(l *leaf) float32 {
	if l.kind == kTable && l.geom != nil {
		return l.geom.w
	}
	v := l.view
	if v == nil || v.diagram != nil || l.kind != kCode && l.kind != kMath && l.kind != kHTML {
		return l.w
	}
	w := l.w
	if v.layout != nil {
		tw, _ := v.layout.Size()
		w = max(w, tw+2*v.tx)
	}
	if v.block != nil {
		w = max(w, v.bx+v.block.w)
	}
	return w
}

func (e *Editor) setBlockHorizontal(l *leaf, x float32) bool {
	maxX := max(blockContentWidth(l)-blockViewport(l).W, 0)
	x = min(max(x, 0), maxX)
	if x == l.horizontal.x {
		return false
	}
	l.horizontal.x = x
	e.leavesLaid = false
	e.invalidate()
	return true
}

func (e *Editor) scrollBlock(ev ui.InputEvent) bool {
	if e.scrollDiagram(ev) {
		if ev.Mods&ui.Shift == 0 && ev.DY != 0 {
			e.scrollDocumentBy(ev.DY)
		}
		return true
	}
	dx := ev.DX
	if dx == 0 && ev.Mods&ui.Shift != 0 {
		dx = ev.DY
	}
	if dx == 0 || e.tableHandleDrag != nil {
		return false
	}
	e.layoutIfNeeded()
	for _, l := range e.leaves {
		if e.foldHidden(l) {
			continue
		}
		box := blockViewport(l)
		if box.Contains(ev.X, ev.Y) && blockContentWidth(l) > box.W {
			e.setBlockHorizontal(l, l.horizontal.x+dx)
			if ev.Mods&ui.Shift == 0 && ev.DY != 0 {
				e.scrollDocumentBy(ev.DY)
			}
			return true
		}
	}
	return false
}

func (e *Editor) scrollDocumentBy(dy float32) {
	e.scroll.Y = min(max(e.scroll.Y+dy, 0), e.scroll.MaxY)
	e.invalidate()
}

// Table controls live in root overlays rather than under ui.Scroll. Route
// their wheel events explicitly to the same owning block/document surface.
func (e *Editor) scrollTableOverlay(t *leaf, ev ui.InputEvent) bool {
	if e.tableHandleDrag != nil {
		e.finishTableHandleDrag(true)
	}
	dx := ev.DX
	if dx == 0 && ev.Mods&ui.Shift != 0 {
		dx = ev.DY
	}
	if dx != 0 && blockContentWidth(t) > blockViewport(t).W {
		e.setBlockHorizontal(t, t.horizontal.x+dx)
		if ev.Mods&ui.Shift == 0 && ev.DY != 0 {
			e.scrollDocumentBy(ev.DY)
		}
		return true
	}
	if ev.DY != 0 {
		e.scrollDocumentBy(ev.DY)
		return true
	}
	return false
}

func (e *Editor) paintBlockHorizontalBar(p *ui.Painter, l *leaf, ox, oy float32) {
	box := blockViewport(l)
	w := blockContentWidth(l)
	if w <= box.W || box.W <= 0 {
		l.horizontal.shown = false
		return
	}
	s := &l.horizontal
	if s.indicator == nil {
		s.indicator = &scrollbar.Indicator{}
	}
	s.shown = s.indicator.Visible(p.Now(), s.x, 0)
	if !s.shown && (e.horizontalDrag == nil || e.horizontalDrag.leaf != l) {
		return
	}
	s.shown = true
	if remaining := s.indicator.Remaining(p.Now()); remaining > 0 {
		p.After(remaining)
	}
	track, thumb := blockHorizontalBarRects(box, w, s.x)
	track.X, track.Y = track.X+ox, track.Y+oy
	thumb.X, thumb.Y = thumb.X+ox, thumb.Y+oy
	e.paintDiagramBar(p, track, thumb, e.horizontalDrag != nil && e.horizontalDrag.leaf == l)
}

func blockHorizontalBarRects(box ui.Rect, contentW, x float32) (track, thumb ui.Rect) {
	track, thumb = diagramBarRects(box, contentW, x)
	track.Y = box.Y + box.H - 7
	thumb.Y = track.Y
	return
}

func (e *Editor) horizontalBarAt(x, y float32) (*leaf, bool) {
	for _, l := range e.leaves {
		if e.foldHidden(l) || !l.horizontal.shown || blockContentWidth(l) <= blockViewport(l).W {
			continue
		}
		track, _ := blockHorizontalBarRects(blockViewport(l), blockContentWidth(l), l.horizontal.x)
		if track.Contains(x, y) {
			return l, true
		}
	}
	return nil, false
}

func (e *Editor) pressBlockHorizontal(l *leaf, x float32) {
	box, w := blockViewport(l), blockContentWidth(l)
	track, thumb := blockHorizontalBarRects(box, w, l.horizontal.x)
	if x < thumb.X || x > thumb.X+thumb.W {
		e.setBlockHorizontal(l, (x-track.X-thumb.W/2)/max(track.W-thumb.W, 1)*(w-box.W))
	}
	e.horizontalDrag = &blockHorizontalDrag{leaf: l, start: x, from: l.horizontal.x}
}

func (e *Editor) dragBlockHorizontal(x float32) {
	d := e.horizontalDrag
	if d == nil {
		return
	}
	box, w := blockViewport(d.leaf), blockContentWidth(d.leaf)
	track, thumb := blockHorizontalBarRects(box, w, d.leaf.horizontal.x)
	e.setBlockHorizontal(d.leaf, d.from+(x-d.start)/max(track.W-thumb.W, 1)*(w-box.W))
}

func (e *Editor) horizontalOwner(off int) *leaf {
	u := e.unitAt(off)
	if u != nil && u.table != nil {
		return u.table
	}
	return u
}

func (e *Editor) revealHorizontalCaret(off int) {
	l := e.horizontalOwner(off)
	if l == nil || blockContentWidth(l) <= blockViewport(l).W {
		return
	}
	r, ok := e.caretRect(off)
	if !ok {
		return
	}
	box := blockViewport(l)
	x := l.horizontal.x
	if r.X < box.X+4 {
		x += r.X - box.X - 4
	} else if r.X+r.W > box.X+box.W-4 {
		x += r.X + r.W - (box.X + box.W) + 4
	}
	e.setBlockHorizontal(l, x)
}
