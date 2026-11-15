package mdedit

import (
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/mdedit/internal/parser"
	"muse/mdoc"
)

// Handle dragging owns reordering only. Text dragging and keyboard structure
// extension keep their existing selection paths and never create this state.
type tableHandleDrag struct {
	lineStart, first, last, index, gap int
	axis                               tableAxis
	startX, startY, position           float32
	moving                             bool
	revision                           uint64
}

func (e *Editor) beginTableHandleDrag(t *leaf, axis tableAxis, index int, shift bool, x, y float32) {
	if t == nil || t.tab == nil || t.geom == nil {
		return
	}
	s := e.tableSelection
	if shift && s != nil && s.lineStart == t.lineStart && s.axis == axis {
		e.selectTableAxis(t, axis, s.anchor, index, false)
	} else if s == nil || s.lineStart != t.lineStart || s.axis != axis || index < s.first || index > s.last {
		e.selectTableAxis(t, axis, index, index, false)
	}
	s = e.tableSelection
	if s == nil {
		return
	}
	e.selectionInclusive, e.selectionLinewise = false, false
	e.modal.line = false
	position := y
	if axis == tableColumns {
		position = x
	}
	e.tableHandleDrag = &tableHandleDrag{lineStart: t.lineStart, first: s.first, last: s.last, index: index, axis: axis, gap: index, startX: x, startY: y, position: position, revision: e.rev}
	e.wantReveal = false
	e.invalidate()
}

func (e *Editor) updateTableHandleDrag(x, y float32) {
	d := e.tableHandleDrag
	if d == nil {
		return
	}
	t := e.tableSelectionLeaf()
	if t == nil || t.geom == nil || t.lineStart != d.lineStart || e.rev != d.revision {
		e.tableHandleDrag = nil
		return
	}
	if math.Abs(float64(x-d.startX))+math.Abs(float64(y-d.startY)) >= 4 {
		d.moving = true
	}
	edges, p := t.geom.rowY, y-t.y
	if d.axis == tableColumns {
		edges, p = t.geom.colX, x-tableContentX(t)
		d.position = max(t.x, min(x, t.x+tableViewportWidth(t)))
	} else {
		d.position = max(t.y, min(y, t.y+t.h))
	}
	d.gap = len(edges) - 1
	for i := 0; i+1 < len(edges); i++ {
		if p < (edges[i]+edges[i+1])/2 {
			d.gap = i
			break
		}
	}
	e.invalidate()
}

func (e *Editor) finishTableHandleDrag(canceled bool) {
	d := e.tableHandleDrag
	e.tableHandleDrag = nil
	if d != nil && !canceled && d.moving && d.revision == e.rev {
		e.reorderTable(d.lineStart, d.axis, d.first, d.last, d.gap)
	}
	e.invalidate()
}

// tableMoveOrder interprets gap in the original grid, before removing the
// selected group. A drop anywhere in that group (including its end) is a no-op.
func tableMoveOrder(n, first, last, gap int) ([]int, int) {
	if first < 0 || first > last || last >= n || gap < 0 || gap > n || gap >= first && gap <= last+1 {
		return nil, first
	}
	remaining := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if i < first || i > last {
			remaining = append(remaining, i)
		}
	}
	at := gap
	if gap > last {
		at -= last - first + 1
	}
	order := append([]int(nil), remaining[:at]...)
	for i := first; i <= last; i++ {
		order = append(order, i)
	}
	order = append(order, remaining[at:]...)
	return order, at
}

func (e *Editor) reorderTable(lineStart int, axis tableAxis, first, last, gap int) bool {
	e.layoutIfNeeded()
	var t *leaf
	for _, l := range e.leaves {
		if l.kind == kTable && l.lineStart == lineStart && l.tab != nil {
			t = l
			break
		}
	}
	if t == nil || axis != tableRows && axis != tableColumns {
		return false
	}
	horizontal := t.horizontal.x
	n := len(t.tab.rows)
	if axis == tableColumns {
		n = t.tab.cols()
	}
	order, at := tableMoveOrder(n, first, last, gap)
	if order == nil {
		return false
	}
	reg := e.tableRegisterForSelection(t, &tableSelection{lineStart: lineStart, axis: tableWhole})
	if reg == nil {
		return false
	}
	if axis == tableRows {
		rows := make([][]string, len(order))
		for i, old := range order {
			rows[i] = reg.rows[old]
		}
		reg.rows = rows
	} else {
		align := make([]parser.Align, len(order))
		for i, old := range order {
			align[i] = alignAt(reg.align, old)
		}
		for r, row := range reg.rows {
			values := make([]string, len(order))
			for i, old := range order {
				if old < len(row) {
					values[i] = row[old]
				}
			}
			reg.rows[r] = values
		}
		reg.align = align
	}
	// The first displayed row remains the GFM header. Reordering changes its
	// contents, not the delimiter's fixed position after that row.
	prefix := e.src[t.lineStart:t.start]
	lines := strings.Split(reg.markdown(), "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = prefix + lines[i]
		} else {
			lines[i] = blankMarkers(prefix) + lines[i]
		}
	}
	value := strings.Join(lines, "\n")
	anchor, active, activeRow, right := first, last, 0, true
	if s := e.tableSelection; s != nil && s.lineStart == lineStart && s.axis == axis {
		anchor, active, activeRow, right = s.anchor, s.active, s.activeRow, s.right
	}
	e.actCaretStill(func() {
		e.begin(stepEdit)
		e.replace(t.lineStart, blockEnd(e.src, t), value)
		e.commit(e.anchor, e.caret)
		for _, l := range e.leaves {
			if l.kind != kTable || l.lineStart != lineStart || l.tab == nil {
				continue
			}
			e.selectTableAxis(l, axis, at, at+last-first, false)
			s := e.tableSelection
			s.anchor = at + max(0, min(anchor-first, last-first))
			s.active = at + max(0, min(active-first, last-first))
			s.right = right
			s.activeRow = activeRow
			if axis == tableRows {
				s.activeRow = s.active
			}
			if h := len(e.hist.undo); h > 0 {
				after := e.tableNavigationState()
				e.hist.undo[h-1].tableAfter = &after
			}
			break
		}
	})
	e.preserveTableHorizontal(lineStart, horizontal)
	return true
}

// Source edits rebuild table leaves. Keep the user's displayed horizontal
// viewport across a control action, clamped if the new table is narrower.
func (e *Editor) preserveTableHorizontal(lineStart int, offset float32) {
	e.layoutIfNeeded()
	for _, l := range e.leaves {
		if l.kind == kTable && l.lineStart == lineStart {
			e.setBlockHorizontal(l, offset)
			e.layoutIfNeeded()
			return
		}
	}
}

func (e *Editor) paintTableHandleDrag(p *ui.Painter, viewport ui.Rect, t *leaf) {
	d := e.tableHandleDrag
	if d == nil || t == nil || t.lineStart != d.lineStart || t.geom == nil {
		return
	}
	x, y := viewport.X+t.x, viewport.Y+t.y-e.scroll.Y
	bg, _ := mdoc.TableControlColors(e.lk.dark)
	ink := e.lk.accent
	p.Clip(viewport, 0, func() {
		var thumb ui.Rect
		if d.axis == tableRows {
			rail := ui.Rect{X: x - 17, Y: y, W: 14, H: t.h}
			p.Fill(rail, bg.Alpha(0.7), 7)
			cy := max(y+14, min(viewport.Y+d.position-e.scroll.Y, y+t.h-14))
			thumb = ui.Rect{X: rail.X + 1, Y: cy - 14, W: 12, H: 28}
		} else {
			rail := ui.Rect{X: x, Y: y - 17, W: tableViewportWidth(t), H: 14}
			p.Fill(rail, bg.Alpha(0.7), 7)
			cx := max(x+14, min(viewport.X+d.position, x+tableViewportWidth(t)-14))
			thumb = ui.Rect{X: cx - 14, Y: rail.Y + 1, W: 28, H: 12}
		}
		p.Fill(thumb, ink, 6)
		for _, shift := range []float32{-4, 0, 4} {
			dot := ui.Rect{X: thumb.X + thumb.W/2 - 1, Y: thumb.Y + thumb.H/2 - 1, W: 2, H: 2}
			if d.axis == tableRows {
				dot.Y += shift
			} else {
				dot.X += shift
			}
			p.Fill(dot, p.Theme().AccentText, 1)
		}
		if d.moving && (d.gap < d.first || d.gap > d.last+1) {
			clip := ui.Rect{X: x - 2, Y: y - 2, W: tableViewportWidth(t) + 4, H: t.h + 4}
			p.Clip(clip, 0, func() {
				if d.axis == tableRows {
					p.Fill(ui.Rect{X: x - 1, Y: y + t.geom.rowY[d.gap] - 2, W: tableViewportWidth(t) + 2, H: 4}, ink, 2)
				} else {
					p.Fill(ui.Rect{X: x + t.geom.colX[d.gap] - t.horizontal.x - 2, Y: y - 1, W: 4, H: t.h + 2}, ink, 2)
				}
			})
		}
	})
}
