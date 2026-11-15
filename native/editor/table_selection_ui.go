package editor

import (
	"muse/locales"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/markdown"
)

// Overlays share a window-level parent. A handle must keep its identity when
// pressing it hides the preceding add controls and cell menus, including when
// multiple tables or editors occupy the same window.
type tableHandleKey struct {
	editor    *Editor
	lineStart int
	axis      tableAxis
	index     int
}

func (e *Editor) tableSelectionHandlesView(c *ui.Context, viewport ui.Rect, pointer ui.Point) {
	for _, t := range e.paintVisibleSpan(e.scroll.Y-32, e.scroll.Y+viewport.H+32) {
		if t.kind != kTable || e.foldHidden(t) || t.tab == nil || t.geom == nil || len(t.geom.rowY) < 2 || len(t.geom.colX) < 2 {
			continue
		}
		for row := range t.tab.rows {
			if t.horizontal.x > 0 {
				continue // the table's real left edge is outside its viewport
			}
			if row+1 >= len(t.geom.rowY) {
				continue
			}
			r := ui.Rect{
				X: viewport.X + t.x - 18,
				Y: viewport.Y + t.y + (t.geom.rowY[row]+t.geom.rowY[row+1])/2 - e.lk.size*1.15/2 - e.scroll.Y,
				W: 14, H: e.lk.size * 1.15,
			}
			if !visibleHandle(r, viewport) {
				continue
			}
			e.tableSelectionHandle(c, viewport, pointer, t, tableRows, row, r, locales.EditorSelectRow(row+1))
		}
		for col := 0; col+1 < len(t.geom.colX); col++ {
			left := max(t.x, tableContentX(t)+t.geom.colX[col])
			right := min(t.x+tableViewportWidth(t), tableContentX(t)+t.geom.colX[col+1])
			if right <= left {
				continue
			}
			r := ui.Rect{
				X: viewport.X + (left+right)/2 - 10,
				Y: viewport.Y + t.y - 18 - e.scroll.Y,
				W: 20, H: 14,
			}
			if !visibleHandle(r, viewport) {
				continue
			}
			e.tableSelectionHandle(c, viewport, pointer, t, tableColumns, col, r, locales.EditorSelectColumn(col+1))
		}
	}
}

func visibleHandle(r, v ui.Rect) bool {
	return r.W > 0 && r.H > 0 && r.X+r.W > v.X && r.X < v.X+v.W && r.Y+r.H > v.Y && r.Y < v.Y+v.H
}

func (e *Editor) tableSelectionHandle(c *ui.Context, viewport ui.Rect, pointer ui.Point, t *leaf, axis tableAxis, index int, r ui.Rect, label string) {
	ui.Overlay(c, func() {
		el := ui.Box(c.Key(tableHandleKey{e, t.lineStart, axis, index})).Absolute().Left(r.X).Top(r.Y).Width(r.W).Height(r.H).
			Label(label).Cursor(ui.CursorPointer).
			HandleInput(func(ev ui.InputEvent) bool {
				switch ev.Kind {
				case ui.InputScroll:
					return e.scrollTableOverlay(t, ev)
				case ui.InputPointerDown:
					if ev.Button != 0 {
						return false
					}
					e.beginTableHandleDrag(t, axis, index, ev.Mods&ui.Shift != 0, r.X+ev.X-viewport.X, r.Y+ev.Y-viewport.Y+e.scroll.Y)
					e.Focus()
					return true
				case ui.InputPointerMove:
					if ev.Button == 0 && e.tableHandleDrag != nil {
						e.updateTableHandleDrag(r.X+ev.X-viewport.X, r.Y+ev.Y-viewport.Y+e.scroll.Y)
						return true
					}
				case ui.InputPointerUp:
					if e.tableHandleDrag != nil {
						if !ev.Canceled {
							e.updateTableHandleDrag(r.X+ev.X-viewport.X, r.Y+ev.Y-viewport.Y+e.scroll.Y)
						}
						e.finishTableHandleDrag(ev.Canceled)
						return true
					}
				}
				return false
			})
		if d := e.tableHandleDrag; d != nil {
			if d.lineStart == t.lineStart && d.axis == axis && d.index == index {
				el.Draw(func(p *ui.Painter, b ui.Rect) { e.paintTableHandleDrag(p, viewport, t) })
			}
			return
		}
		_, _, over := el.PointerPosition()
		hoverRow, hoverCol := tableHoverAxes(t, pointer.X, pointer.Y)
		hot := axis == tableRows && hoverRow == index || axis == tableColumns && hoverCol == index
		if over || hot {
			c.After(40 * time.Millisecond)
			el.Draw(func(p *ui.Painter, b ui.Rect) {
				bg, ink := markdown.TableControlColors(e.lk.dark)
				col := e.lk.accent.Alpha(0.5)
				cx, cy := b.X+b.W/2, b.Y+b.H/2
				if axis == tableRows {
					// The edge marker shares the grid's X. The active caret
					// remains a full-height stroke; this marker stays short.
					p.Fill(ui.Rect{X: viewport.X + t.x - 1.5, Y: cy - 9, W: 3, H: 18}, col, 1.5)
					if over {
						p.Fill(ui.Rect{X: cx - 5, Y: cy - 12, W: 10, H: 24}, bg, 5)
						for _, dy := range []float32{-5, 0, 5} {
							p.Fill(ui.Rect{X: cx - 1, Y: cy + dy - 1, W: 2, H: 2}, ink, 1)
						}
					}
				} else {
					p.Fill(ui.Rect{X: cx - 10, Y: viewport.Y + t.y - e.scroll.Y - 1.5, W: 20, H: 3}, col, 1.5)
					if over {
						p.Fill(ui.Rect{X: cx - 12, Y: cy - 5, W: 24, H: 10}, bg, 5)
						for _, dx := range []float32{-5, 0, 5} {
							p.Fill(ui.Rect{X: cx + dx - 1, Y: cy - 1, W: 2, H: 2}, ink, 1)
						}
					}
				}
			})
		}
	})
}

// tableCellIndexAt uses the shaped grid, including wrapped row heights. These
// are document-element coordinates; viewport scroll is applied by the caller.
func tableCellIndexAt(t *leaf, x, y float32) (row, col int, ok bool) {
	if t == nil || t.geom == nil || t.tab == nil || len(t.geom.rowY) < 2 || len(t.geom.colX) < 2 {
		return -1, -1, false
	}
	x, y = x-t.x, y-t.y
	if x < 0 || y < 0 || x >= tableViewportWidth(t) || y >= t.geom.rowY[len(t.geom.rowY)-1] {
		return -1, -1, false
	}
	x += t.horizontal.x
	return tableRowIndexAt(t, t.y+y), tableColumnIndexAt(t, x), true
}

// Only exposed cell edges produce hover affordances: first column => left,
// first row => top, top-left => both. Structural selection painting is
// independent of this pointer-only projection.
func tableHoverAxes(t *leaf, x, y float32) (row, col int) {
	r, c, ok := tableCellIndexAt(t, x, y)
	row, col = -1, -1
	if ok && c == 0 && t.horizontal.x == 0 {
		row = r
	}
	if ok && r == 0 {
		col = c
	}
	return row, col
}

func tableRowIndexAt(t *leaf, y float32) int {
	if t == nil || t.geom == nil || len(t.geom.rowY) < 2 {
		return 0
	}
	y -= t.y
	for i := 0; i+1 < len(t.geom.rowY); i++ {
		if y < t.geom.rowY[i+1] {
			return min(i, len(t.tab.rows)-1)
		}
	}
	return max(len(t.tab.rows)-1, 0)
}

func tableColumnIndexAt(t *leaf, x float32) int {
	if t == nil || t.geom == nil || len(t.geom.colX) < 2 {
		return 0
	}
	for i := 0; i+1 < len(t.geom.colX); i++ {
		if x < t.geom.colX[i+1] {
			return min(i, t.tab.cols()-1)
		}
	}
	return max(t.tab.cols()-1, 0)
}
