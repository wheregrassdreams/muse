package mdedit

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
)

func (e *Editor) tableSelectionHandlesView(c *ui.Context, viewport ui.Rect) {
	for _, t := range e.leaves {
		if t.kind != kTable || t.tab == nil || t.geom == nil || len(t.geom.rowY) < 2 || len(t.geom.colX) < 2 {
			continue
		}
		for row := range t.tab.rows {
			if row+1 >= len(t.geom.rowY) {
				continue
			}
			r := ui.Rect{
				X: viewport.X + t.x - 24,
				Y: viewport.Y + t.y + (t.geom.rowY[row]+t.geom.rowY[row+1])/2 - e.lk.size*0.55 - e.scroll.Y,
				W: 20, H: e.lk.size * 1.15,
			}
			if !visibleHandle(r, viewport) {
				continue
			}
			selected := e.tableSelection != nil && e.tableSelection.lineStart == t.lineStart && e.tableSelection.axis == tableRows && row >= e.tableSelection.first && row <= e.tableSelection.last
			e.tableSelectionHandle(c, viewport, t, tableRows, row, r, "⋮", "选择第 %d 行", selected)
		}
		for col := 0; col+1 < len(t.geom.colX); col++ {
			r := ui.Rect{
				X: viewport.X + t.x + (t.geom.colX[col]+t.geom.colX[col+1])/2 - 10,
				Y: viewport.Y + t.y - e.lk.size*1.3 - e.scroll.Y,
				W: 20, H: e.lk.size * 1.15,
			}
			if !visibleHandle(r, viewport) {
				continue
			}
			selected := e.tableSelection != nil && e.tableSelection.lineStart == t.lineStart && e.tableSelection.axis == tableColumns && col >= e.tableSelection.first && col <= e.tableSelection.last
			e.tableSelectionHandle(c, viewport, t, tableColumns, col, r, "⋯", "选择第 %d 列", selected)
		}
	}
}

func visibleHandle(r, v ui.Rect) bool {
	return r.W > 0 && r.H > 0 && r.X+r.W > v.X && r.X < v.X+v.W && r.Y+r.H > v.Y && r.Y < v.Y+v.H
}

func (e *Editor) tableSelectionHandle(c *ui.Context, viewport ui.Rect, t *leaf, axis tableAxis, index int, r ui.Rect, glyph, label string, selected bool) {
	ui.Overlay(c, func() {
		el := ui.Box(c).Absolute().Left(r.X).Top(r.Y).Width(r.W).Height(r.H).
			Label(labelForIndex(label, index+1)).Cursor(ui.CursorPointer).
			HandleInput(func(ev ui.InputEvent) bool {
				switch ev.Kind {
				case ui.InputPointerDown:
					if ev.Button != 0 {
						return false
					}
					e.beginTableSelectionDrag(t, axis, index, ev.Mods&ui.Shift != 0)
					e.Focus()
					return true
				case ui.InputPointerMove:
					if ev.Button == 0 && e.tableSelectDrag != nil {
						if axis == tableRows {
							y := r.Y + ev.Y - viewport.Y + e.scroll.Y
							e.updateTableSelectionDrag(tableRowIndexAt(t, y))
						} else {
							x := r.X + ev.X - viewport.X
							e.updateTableSelectionDrag(tableColumnIndexAt(t, x-t.x))
						}
						return true
					}
				case ui.InputPointerUp:
					if e.tableSelectDrag != nil {
						e.tableSelectDrag = nil
						return true
					}
				}
				return false
			})
		_, _, over := el.PointerPosition()
		if over || selected || e.tableSelectDrag != nil && e.tableSelectDrag.lineStart == t.lineStart && e.tableSelectDrag.axis == axis {
			if over {
				c.After(40 * time.Millisecond)
			}
			el.Draw(func(p *ui.Painter, b ui.Rect) {
				col := e.lk.muted.Alpha(0.7)
				if selected {
					col = e.lk.accent.Alpha(0.85)
				}
				p.Fill(b, e.lk.panel, b.H/3)
				p.RichText(b.X+b.W/2-5, b.Y+b.H/2-e.lk.size*0.45, 20,
					ui.Span{Text: glyph, Size: e.lk.size, Color: col})
			})
		}
	})
}

func labelForIndex(format string, oneBased int) string {
	return fmt.Sprintf(format, oneBased)
}

func tableRowIndexAt(t *leaf, y float32) int {
	if t == nil || t.geom == nil || len(t.geom.rowY) < 2 {
		return 0
	}
	y -= t.y
	for i := 0; i+1 < len(t.geom.rowY); i++ {
		if y < (t.geom.rowY[i]+t.geom.rowY[i+1])/2 {
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
		if x < (t.geom.colX[i]+t.geom.colX[i+1])/2 {
			return min(i, t.tab.cols()-1)
		}
	}
	return max(t.tab.cols()-1, 0)
}
