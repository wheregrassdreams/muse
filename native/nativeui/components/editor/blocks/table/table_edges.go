package editor

import (
	"time"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/theme"
)

type tableEdgeKey struct {
	editor    *Editor
	lineStart int
	column    bool
}

// tableEdgeEdit targets the displayed table, never the unrelated caret.
// replace maps both selection endpoints; the entire operation is one undo step.
func (e *Editor) tableEdgeEdit(t *leaf, column bool) {
	if t == nil || t.tab == nil {
		return
	}
	e.actCaretStill(func() {
		if column {
			_, _ = e.tableController.AddColumnRight(editorTableHost{editor: e}, t.lineStart)
			return
		}
		_, _ = e.tableController.AddRowBelow(editorTableHost{editor: e}, t.lineStart)
	})
}

func (e *Editor) tableEdgesView(c *ui.Context, viewport ui.Rect, pointer ui.Point) {
	// A text drag keeps the canvas as its hit surface; hover controls must
	// not intercept its next move or release at a cell/border.
	if e.dragging {
		return
	}
	if e.tableHandleDrag != nil {
		e.tableSelectionHandlesView(c, viewport, pointer)
		return
	}
	for _, t := range e.paintVisibleSpan(e.scroll.Y-32, e.scroll.Y+viewport.H+32) {
		if t.kind != kTable || e.foldHidden(t) {
			continue
		}
		if t.geom == nil || t.tab == nil || len(t.geom.rowY) == 0 {
			continue
		}
		x, y := viewport.X+tableContentX(t), viewport.Y+t.y-e.scroll.Y
		w, h := t.geom.w, t.geom.rowY[len(t.geom.rowY)-1]
		for _, column := range []bool{false, true} {
			r := ui.Rect{X: x, Y: y + h - 6, W: w, H: 24}
			label := locales.EditorAddRowBelow()
			if column {
				if x+w > viewport.X+t.x+tableViewportWidth(t)+1 {
					continue
				}
				r = ui.Rect{X: x + w - 6, Y: y, W: 24, H: h}
				label = locales.EditorAddColumnRight()
			}
			trigger := r
			// Only the visible capsule takes clicks. The gap between it and
			// the grid remains available to the native boundary caret.
			if column {
				r.X += 10
				r.W -= 10
			} else {
				r.Y += 10
				r.H -= 10
				right := min(r.X+r.W, viewport.X+t.x+tableViewportWidth(t))
				r.X = max(r.X, viewport.X+t.x)
				r.W = right - r.X
			}
			// Clip the hit strip as well as its drawing to the visible editor.
			right, bottom := min(r.X+r.W, viewport.X+viewport.W), min(r.Y+r.H, viewport.Y+viewport.H)
			r.X, r.Y = max(r.X, viewport.X), max(r.Y, viewport.Y)
			r.W, r.H = right-r.X, bottom-r.Y
			if r.W <= 0 || r.H <= 0 {
				continue
			}
			ui.Overlay(c, func() {
				el := ui.Box(c.Key(tableEdgeKey{e, t.lineStart, column})).Absolute().Left(r.X).Top(r.Y).Width(r.W).Height(r.H).Label(label).Cursor(ui.CursorPointer).Tooltip(label).
					HandleInput(func(ev ui.InputEvent) bool {
						if ev.Kind != ui.InputScroll {
							return false
						}
						return e.scrollTableOverlay(t, ev)
					})
				_, _, over := el.PointerPosition()
				near := trigger.Contains(viewport.X+pointer.X, viewport.Y+pointer.Y-e.scroll.Y)
				if over || near {
					c.After(40 * time.Millisecond)
					el.Draw(func(p *ui.Painter, b ui.Rect) {
						colors := theme.MarkdownColors(e.lk.dark)
						bg, ink := uiMarkdownColor(colors.TableControlBackground), uiMarkdownColor(colors.TableControlInk)
						p.Fill(b, bg, min(b.W, b.H)/2)
						cx, cy := b.X+b.W/2, b.Y+b.H/2
						p.Fill(ui.Rect{X: cx - 4, Y: cy - 1, W: 8, H: 2}, ink, 1)
						p.Fill(ui.Rect{X: cx - 1, Y: cy - 4, W: 2, H: 8}, ink, 1)
					})
				}
				if el.Clicked() {
					e.tableEdgeEdit(t, column)
					e.Focus()
				}
			})
		}
	}
	e.tableSelectionHandlesView(c, viewport, pointer)
	e.tableCellMenusView(c, viewport, pointer)
}
