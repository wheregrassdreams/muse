package mdedit

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
	"muse/mdoc"
)

type tableCellMenuKey struct {
	editor              *Editor
	lineStart, row, col int
}

// Table overlays target their displayed table/column, independently of the
// text caret. Markdown alignment is column-wide, as the menu labels state.
func (e *Editor) tableCellMenusView(c *ui.Context, viewport ui.Rect, pointer ui.Point) {
	if e.dragging || e.tableHandleDrag != nil {
		return
	}
	for _, t := range e.leaves {
		if t.kind != kTable || e.foldHidden(t) || t.tab == nil || t.geom == nil {
			continue
		}
		for row, cells := range t.tab.rows {
			if row+1 >= len(t.geom.rowY) {
				break
			}
			for col := range cells {
				if col+1 >= len(t.geom.colX) {
					break
				}
				right := tableContentX(t) + t.geom.colX[col+1]
				if right > t.x+tableViewportWidth(t) || right-12 < t.x {
					continue
				}
				top, bottom := t.y+t.geom.rowY[row], t.y+t.geom.rowY[row+1]
				r := ui.Rect{X: viewport.X + right - 12, Y: viewport.Y + (top+bottom)/2 - 12 - e.scroll.Y, W: 12, H: 24}
				if !visibleHandle(r, viewport) {
					continue
				}
				ui.Overlay(c, func() {
					button := ui.ButtonBase(c.Key(tableCellMenuKey{e, t.lineStart, row, col})).Absolute().Left(r.X).Top(r.Y).Width(r.W).Height(r.H).Padding(0).Radius(5).
						Label(fmt.Sprintf("第%d行第%d列对齐", row+1, col+1)).Tooltip("列对齐")
					button.HandleInput(func(ev ui.InputEvent) bool {
						if ev.Kind == ui.InputScroll {
							return e.scrollTableOverlay(t, ev)
						}
						if ev.Kind == ui.InputPointerMove {
							e.hover(r.X+ev.X-viewport.X, r.Y+ev.Y-viewport.Y+e.scroll.Y)
						}
						return false
					})
					button.Menu(func(m *ui.Menu) {
						align := parser.AlignNone
						if col < len(t.tab.align) {
							align = t.tab.align[col]
						}
						for _, item := range []struct {
							label, cmd string
							align      parser.Align
						}{{"左对齐（此列）", "alignLeft", parser.AlignLeft}, {"居中（此列）", "alignCenter", parser.AlignCenter}, {"右对齐（此列）", "alignRight", parser.AlignRight}} {
							checked := align == item.align || item.align == parser.AlignLeft && align == parser.AlignNone
							if m.Item(item.label).Checked(checked).Chosen() {
								e.tableColumnAlignment(t.lineStart, col, item.cmd)
							}
						}
					})
					hoverRow, hoverCol, inside := tableCellIndexAt(t, pointer.X, pointer.Y)
					hot := inside && hoverRow == row && hoverCol == col
					if button.Hovered() || hot {
						c.After(40 * time.Millisecond)
						button.Draw(func(p *ui.Painter, b ui.Rect) {
							_, ink := mdoc.TableControlColors(e.lk.dark)
							if button.Hovered() {
								bg, _ := mdoc.TableControlColors(e.lk.dark)
								p.Fill(b, bg, 5)
							}
							for _, dy := range []float32{-4, 0, 4} {
								p.Fill(ui.Rect{X: b.X + b.W/2 - 1, Y: b.Y + b.H/2 + dy - 1, W: 2, H: 2}, ink, 1)
							}
						})
					}
				})
			}
		}
	}
}

func (e *Editor) tableColumnAlignment(lineStart, col int, cmd string) {
	e.finishBlockInput()
	e.layoutIfNeeded()
	for _, t := range e.leaves {
		if t.kind == kTable && t.lineStart == lineStart && t.tab != nil && col >= 0 && col < t.tab.cols() {
			horizontal := t.horizontal.x
			e.actCaretStill(func() { e.alignColumn(t, e.tableLines(t), col, cmd) })
			e.preserveTableHorizontal(lineStart, horizontal)
			e.Focus()
			return
		}
	}
}
