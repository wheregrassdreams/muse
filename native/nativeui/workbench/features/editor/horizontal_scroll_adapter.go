package editor

import (
	"github.com/egoist/mygo/ui"
	editorlayout "github.com/wheregrassdreams/muse-editor/text/layout"
)

func policyRect(rect ui.Rect) editorlayout.Rect {
	return editorlayout.Rect{X: rect.X, Y: rect.Y, W: rect.W, H: rect.H}
}

func toolkitRect(rect editorlayout.Rect) ui.Rect {
	return ui.Rect{X: rect.X, Y: rect.Y, W: rect.W, H: rect.H}
}

func horizontalBarRects(box ui.Rect, contentWidth, offset float32) (ui.Rect, ui.Rect) {
	track, thumb := editorlayout.HorizontalBarRects(policyRect(box), contentWidth, offset)
	return toolkitRect(track), toolkitRect(thumb)
}

type blockHorizontalDrag struct {
	leaf *leaf
	drag editorlayout.HorizontalDrag
}

func tableViewportWidth(t *leaf) float32 {
	if t == nil || t.geom == nil {
		return 0
	}
	return min(t.w, t.geom.w)
}

func tableContentX(t *leaf) float32 { return t.x - t.horizontal.Offset }

func blockScrollOffset(u *leaf) float32 {
	if u == nil || u.table != nil {
		return 0 // cell.x already includes the table's scroll
	}
	return u.horizontal.Offset
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
	if v == nil || v.diagramModel != nil || l.kind != kCode && l.kind != kMath && l.kind != kHTML {
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
	x = editorlayout.ClampOffset(blockContentWidth(l), blockViewport(l).W, x)
	if x == l.horizontal.Offset {
		return false
	}
	l.horizontal.Offset = x
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
			e.setBlockHorizontal(l, editorlayout.ScrollOffset(horizontalFrame(l), dx))
			if ev.Mods&ui.Shift == 0 && ev.DY != 0 {
				e.scrollDocumentBy(ev.DY)
			}
			return true
		}
	}
	return false
}

// diagramAt returns an overflowing diagram under the pointer, using editor
// local coordinates like the diagram paint and hit-test paths.
func (e *Editor) diagramAt(x, y float32) (*leaf, *leafView, ui.Rect, float32, bool) {
	for _, l := range e.leaves {
		v := l.view
		if v == nil || v.diagramModel == nil {
			continue
		}
		box, contentW := e.diagramBox(l, v, 0, 0)
		if contentW > box.W && box.Contains(x, y) {
			return l, v, box, contentW, true
		}
	}
	return nil, nil, ui.Rect{}, 0, false
}

// scrollDiagram scrolls an overflowing diagram sideways; the caller keeps
// the vertical component available to the document viewport.
func (e *Editor) scrollDiagram(ev ui.InputEvent) bool {
	_, v, box, contentW, ok := e.diagramAt(ev.X, ev.Y)
	if !ok {
		return false
	}
	dx := ev.DX
	if dx == 0 && ev.Mods&ui.Shift != 0 {
		dx = ev.DY
	}
	if dx == 0 {
		return false
	}
	v.diagX = min(max(v.diagX+dx, 0), contentW-box.W)
	e.invalidate()
	return true
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
		e.setBlockHorizontal(t, editorlayout.ScrollOffset(horizontalFrame(t), dx))
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
		l.horizontal.Shown = false
		return
	}
	s := &l.horizontal
	s.Shown = s.Indicator.Visible(p.Now(), s.Offset, 0)
	if !s.Shown && (e.horizontalDrag == nil || e.horizontalDrag.leaf != l) {
		return
	}
	s.Shown = true
	if remaining := s.Indicator.Remaining(p.Now()); remaining > 0 {
		p.After(remaining)
	}
	track, thumb := horizontalBarRects(box, w, s.Offset)
	track.X, track.Y = track.X+ox, track.Y+oy
	thumb.X, thumb.Y = thumb.X+ox, thumb.Y+oy
	e.paintDiagramBar(p, track, thumb, e.horizontalDrag != nil && e.horizontalDrag.leaf == l)
}

func (e *Editor) horizontalBarAt(x, y float32) (*leaf, bool) {
	for _, l := range e.leaves {
		if e.foldHidden(l) || !l.horizontal.Shown || blockContentWidth(l) <= blockViewport(l).W {
			continue
		}
		track, _ := horizontalBarRects(blockViewport(l), blockContentWidth(l), l.horizontal.Offset)
		if track.Contains(x, y) {
			return l, true
		}
	}
	return nil, false
}

func (e *Editor) pressBlockHorizontal(l *leaf, x float32) {
	drag, offset := editorlayout.BeginHorizontalDrag(horizontalFrame(l), x)
	e.setBlockHorizontal(l, offset)
	e.horizontalDrag = &blockHorizontalDrag{leaf: l, drag: drag}
}

func (e *Editor) dragBlockHorizontal(x float32) {
	d := e.horizontalDrag
	if d == nil {
		return
	}
	e.setBlockHorizontal(d.leaf, editorlayout.DragHorizontal(horizontalFrame(d.leaf), d.drag, x))
}

func horizontalFrame(l *leaf) editorlayout.HorizontalFrame {
	return editorlayout.HorizontalFrame{Viewport: policyRect(blockViewport(l)), ContentWidth: blockContentWidth(l), Offset: l.horizontal.Offset}
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
	x := l.horizontal.Offset
	x = editorlayout.RevealOffset(x, box.X, box.W, r.X, r.X+r.W, 4)
	e.setBlockHorizontal(l, x)
}
