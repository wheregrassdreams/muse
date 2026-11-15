package editor

import (
	"muse/locales"
	"time"

	"github.com/egoist/mygo/ui"
	syntax "muse/markdown/syntax"
	"muse/theme"
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
	for _, t := range e.paintVisibleSpan(e.scroll.Y-32, e.scroll.Y+viewport.H+32) {
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
						Label(locales.EditorTableCellAlignment(row+1, col+1)).Tooltip(locales.EditorTableColumnAlignment())
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
						align := syntax.IncrementalAlignNone
						if col < len(t.tab.align) {
							align = t.tab.align[col]
						}
						for _, item := range []struct {
							label, cmd string
							align      syntax.IncrementalAlign
						}{{locales.EditorAlignLeftColumn(), "alignLeft", syntax.IncrementalAlignLeft}, {locales.EditorAlignCenterColumn(), "alignCenter", syntax.IncrementalAlignCenter}, {locales.EditorAlignRightColumn(), "alignRight", syntax.IncrementalAlignRight}} {
							checked := align == item.align || item.align == syntax.IncrementalAlignLeft && align == syntax.IncrementalAlignNone
							if m.Item(item.label).Checked(checked).Chosen() {
								e.focusTableMenuCell(t, row, col)
								e.tableColumnAlignment(t.lineStart, col, item.cmd)
							}
						}
					})
					hoverRow, hoverCol, inside := tableCellIndexAt(t, pointer.X, pointer.Y)
					hot := inside && hoverRow == row && hoverCol == col
					if button.Hovered() || hot {
						c.After(40 * time.Millisecond)
						button.Draw(func(p *ui.Painter, b ui.Rect) {
							ink := uiMarkdownColor(theme.MarkdownColors(e.lk.dark).TableControlInk)
							if button.Hovered() {
								bg := uiMarkdownColor(theme.MarkdownColors(e.lk.dark).TableControlBackground)
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

// focusTableMenuCell gives menu actions the same table-cell focus semantics as
// Vim and pointer navigation. An existing region containing the menu cell is
// retained; choosing a menu outside it moves the active region to that cell.
func (e *Editor) focusTableMenuCell(t *leaf, row, col int) {
	// An ordinary text selection belongs to the document text layer. Do not
	// replace it with a structural cell selection just because a table menu ran.
	if e.anchor != e.caret && e.tableSelection == nil && e.tableBridge == nil {
		return
	}
	if e.tableSelection != nil && e.tableSelection.lineStart == t.lineStart && e.tableSelectionContainsCell(t, row, col) {
		return
	}
	e.selectTableCellRegion(t, row, col, row, col)
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
