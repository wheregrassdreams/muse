package mdedit

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"time"
)

// tableEdgeEdit targets the displayed table, never the unrelated caret.
// replace maps both selection endpoints; the entire operation is one undo step.
func (e *Editor) tableEdgeEdit(t *leaf, column bool) {
	e.actCaretStill(func() {
		if column {
			lines := e.tableLines(t)
			e.begin(stepEdit)
			for k := len(lines) - 1; k >= 0; k-- {
				pipes := splitRow(e.src, lines[k])
				if len(pipes) == 0 {
					continue
				}
				text := "   |"
				if k == 1 {
					text = " --- |"
				}
				at := lines[k][1]
				if strings.TrimSpace(e.src[pipes[len(pipes)-1]+1:at]) != "" {
					text = " |" + text
				}
				e.replace(at, at, text)
			}
			e.commit(e.anchor, e.caret)
			return
		}
		lines := e.tableLines(t)
		if len(lines) < 2 {
			return
		}
		at := lines[len(lines)-1][1]
		prefix := blankMarkers(e.src[t.lineStart:t.start])
		e.begin(stepEdit)
		e.replace(at, at, "\n"+prefix+"|"+strings.Repeat("   |", t.tab.cols()))
		e.commit(e.anchor, e.caret)
	})
}

func (e *Editor) tableEdgesView(c *ui.Context, viewport ui.Rect) {
	for _, t := range e.leaves {
		if t.geom == nil || t.tab == nil || len(t.geom.rowY) == 0 {
			continue
		}
		x, y := viewport.X+t.x, viewport.Y+t.y-e.scroll.Y
		w, h := t.geom.w, t.geom.rowY[len(t.geom.rowY)-1]
		for _, column := range []bool{false, true} {
			r := ui.Rect{X: x, Y: y + h - 6, W: w, H: 24}
			label := "在下方新增行"
			if column {
				r = ui.Rect{X: x + w - 6, Y: y, W: 24, H: h}
				label = "在右侧新增列"
			}
			// Clip the hit strip as well as its drawing to the visible editor.
			right, bottom := min(r.X+r.W, viewport.X+viewport.W), min(r.Y+r.H, viewport.Y+viewport.H)
			r.X, r.Y = max(r.X, viewport.X), max(r.Y, viewport.Y)
			r.W, r.H = right-r.X, bottom-r.Y
			if r.W <= 0 || r.H <= 0 {
				continue
			}
			ui.Overlay(c, func() {
				el := ui.Box(c).Absolute().Left(r.X).Top(r.Y).Width(r.W).Height(r.H).Label(label).Cursor(ui.CursorPointer).Tooltip(label)
				_, _, over := el.PointerPosition()
				if over {
					c.After(40 * time.Millisecond)
					el.Draw(func(p *ui.Painter, b ui.Rect) {
						p.Fill(b, p.Theme().Background, 0)
						p.Stroke(b, e.lk.border, 0, 1)
						p.RichText(b.X+b.W/2-5, b.Y+b.H/2-10, 20, ui.Span{Text: "+", Size: 20, Color: e.lk.muted})
					})
				}
				if el.Clicked() {
					e.tableEdgeEdit(t, column)
					e.Focus()
				}
			})
		}
	}
}
