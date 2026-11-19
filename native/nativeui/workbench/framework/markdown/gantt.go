package markdown

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wheregrassdreams/muse-editor/markdown/diagrams"
)

func renderGantt(c *ui.Context, chart *diagrams.GanttChart) {
	t := c.Theme()
	ui.Box(c).FillWidth().Height(chart.Height()).Draw(func(p *ui.Painter, r ui.Rect) { drawGantt(p, r, t, chart) })
}

func drawGantt(p *ui.Painter, r ui.Rect, t *ui.Theme, g *diagrams.GanttChart) {
	y := r.Y + 12
	if g.Title != "" {
		sp := ui.Span{Text: g.Title, Size: 13, Weight: 650, Color: t.Text}
		p.RichText(r.X+16, y, 0, sp)
		y += 24
	}
	labelW := float32(160)
	timelineX := r.X + labelW
	timelineW := r.W - labelW - 20
	if timelineW < 120 {
		timelineW = 120
	}
	days := float32(g.End.Sub(g.Start).Hours() / 24)
	if days <= 0 {
		days = 1
	}
	pxPerDay := timelineW / days

	for i, ticks := 0, 5; i <= ticks; i++ {
		x := timelineX + timelineW*float32(i)/float32(ticks)
		p.Line(x, y, x, y+float32(len(g.Rows))*26+4, 1, t.Border)
		label := g.Start.Add(time.Duration(float64(days) * float64(i) / float64(ticks) * float64(24*time.Hour))).Format("01-02")
		sp := ui.Span{Text: label, Size: 9.5, Color: t.TextMuted}
		p.RichText(x+3, y-16, 0, sp)
	}
	y += 8

	rowY := y
	for _, row := range g.Rows {
		if row.Task == nil {
			sp := ui.Span{Text: row.Section, Size: 11, Weight: 650, Color: t.Text}
			p.RichText(r.X+16, rowY+5, labelW-24, sp)
			rowY += 26
			continue
		}
		task := row.Task
		sp := ui.Span{Text: task.Name, Size: 10.5, Color: t.Text}
		p.RichText(r.X+24, rowY+6, labelW-32, sp)
		offset := float32(task.Start.Sub(g.Start).Hours() / 24)
		x := timelineX + offset*pxPerDay
		w := float32(task.Duration.Hours()/24) * pxPerDay
		if task.Milestone {
			path := ui.Path{}
			path.MoveTo(x+5, rowY+5)
			path.LineTo(x+10, rowY+13)
			path.LineTo(x+5, rowY+21)
			path.LineTo(x, rowY+13)
			path.Close()
			p.FillPath(&path, t.TextMuted)
			rowY += 26
			continue
		}
		if w < 4 {
			w = 4
		}
		color := ui.Hex(piePalette[0])
		if task.Done {
			color = ui.Hex("#6aa84f")
		} else if task.Active {
			color = ui.Hex("#4f7cff")
		} else if task.Critical {
			color = ui.Hex("#e5679b")
		}
		p.Fill(ui.Rect{X: x, Y: rowY + 7, W: w, H: 12}, color, 3)
		rowY += 26
	}
}
