package markdown

import (
	"github.com/egoist/mygo/ui"
	"muse/markdown/diagrams"
)

func renderSequence(c *ui.Context, graph *diagrams.SequenceDiagram) {
	t := c.Theme()
	ui.Box(c).FillWidth().Height(graph.Height()).Draw(func(p *ui.Painter, r ui.Rect) { drawSequence(p, r, t, graph) })
}

func drawSequence(p *ui.Painter, r ui.Rect, t *ui.Theme, graph *diagrams.SequenceDiagram) {
	positions := graph.Layout(r.W)
	byID := make(map[string]diagrams.SequencePosition, len(graph.Participants))
	top := r.Y + 12
	for i, part := range graph.Participants {
		pos := positions[i]
		pos.X += r.X
		byID[part.ID] = pos
		box := ui.Rect{X: pos.X - pos.Width/2, Y: top, W: pos.Width, H: 30}
		p.Fill(box, t.Surface, 6)
		p.Stroke(box, t.Border, 6, 1)
		sp := ui.Span{Text: part.Label, Size: 12, Color: t.Text}
		tw, th := p.MeasureText(box.W-12, sp)
		p.RichText(box.X+(box.W-tw)/2, box.Y+(box.H-th)/2, box.W-12, sp)
		for y := box.Y + box.H + 6; y < r.Y+r.H-12; y += 7 {
			p.Line(pos.X, y, pos.X, y+3, 1, t.Border)
		}
	}
	y := top + 54
	for _, event := range graph.Events {
		from, fromOK := byID[event.From]
		to, toOK := byID[event.To]
		if !fromOK || !toOK {
			continue
		}
		if event.Kind == diagrams.SequenceNote {
			x0, x1 := from.X, to.X
			if x0 > x1 {
				x0, x1 = x1, x0
			}
			box := ui.Rect{X: x0 - 40, Y: y, W: x1 - x0 + 80, H: 28}
			p.Fill(box, t.Surface, 5)
			p.Stroke(box, t.Border, 5, 1)
			sp := ui.Span{Text: event.Text, Size: 10.5, Color: t.Text}
			tw, th := p.MeasureText(box.W-12, sp)
			p.RichText(box.X+(box.W-tw)/2, box.Y+(box.H-th)/2, box.W-12, sp)
			y += 34
			continue
		}
		drawSeqMessage(p, t, from.X, to.X, y, event.Text, event.Dashed)
		y += 42
	}
}
func drawSeqMessage(p *ui.Painter, t *ui.Theme, x1, x2, y float32, text string, dashed bool) {
	color := t.TextMuted
	if x1 == x2 {
		// Self message: a small loop.
		p.Line(x1, y, x1+24, y, 1.3, color)
		p.Line(x1+24, y, x1+24, y+10, 1.3, color)
		p.Line(x2+11, y+10, x2, y+10, 1.3, color)
		arrowRight(p, x2, y+10, color)
		sp := ui.Span{Text: text, Size: 10.5, Color: t.Text}
		p.RichText(x1+30, y-14, 0, sp)
		return
	}
	dir := float32(1)
	if x2 < x1 {
		dir = -1
	}
	if dashed {
		for x := x1; (dir > 0 && x < x2) || (dir < 0 && x > x2); x += dir * 8 {
			end := x + dir*4
			p.Line(x, y, end, y, 1.3, color)
		}
	} else {
		p.Line(x1, y, x2, y, 1.3, color)
	}
	if dir > 0 {
		arrowRight(p, x2, y, color)
	} else {
		arrowLeft(p, x2, y, color)
	}
	sp := ui.Span{Text: text, Size: 10.5, Color: t.Text}
	tw, _ := p.MeasureText(0, sp)
	p.RichText((x1+x2)/2-tw/2, y-15, 0, sp)
}

func arrowRight(p *ui.Painter, x, y float32, c ui.Color) {
	p.Line(x, y, x-7, y-4, 1.3, c)
	p.Line(x, y, x-7, y+4, 1.3, c)
}

func arrowLeft(p *ui.Painter, x, y float32, c ui.Color) {
	p.Line(x, y, x+7, y-4, 1.3, c)
	p.Line(x, y, x+7, y+4, 1.3, c)
}
