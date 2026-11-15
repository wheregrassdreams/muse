package markdown

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
	"muse/markdown/diagrams"
)

var piePalette = []string{
	"#4f7cff", "#2bb5a0", "#f0a43c", "#9b6cff",
	"#e5679b", "#6aa84f", "#5aa9e6", "#d97757",
}

func renderPie(c *ui.Context, chart *diagrams.PieChart) {
	t := c.Theme()
	ui.Box(c).FillWidth().Height(286).Draw(func(p *ui.Painter, r ui.Rect) { drawPie(p, r, t, chart) })
}

func drawPie(p *ui.Painter, r ui.Rect, t *ui.Theme, chart *diagrams.PieChart) {
	titleH := float32(0)
	if chart.Title != "" {
		titleH = 26
		sp := ui.Span{Text: chart.Title, Size: 13, Color: t.Text}
		tw, _ := p.MeasureText(0, sp)
		p.RichText(r.X+(r.W-tw)/2, r.Y+8, 0, sp)
	}
	total := 0.0
	for _, s := range chart.Slices {
		total += s.Value
	}
	radius := float32(math.Min(92, float64((r.H-titleH-28)/2)))
	cx := r.X + radius + 44
	cy := r.Y + titleH + (r.H-titleH)/2
	start := float32(-math.Pi / 2)
	for i, s := range chart.Slices {
		sweep := float32(s.Value/total) * 2 * float32(math.Pi)
		path := ui.Path{}
		path.MoveTo(cx, cy)
		path.LineTo(cx+float32(math.Cos(float64(start)))*radius, cy+float32(math.Sin(float64(start)))*radius)
		steps := int(math.Ceil(float64(sweep) / (math.Pi / 30)))
		if steps < 1 {
			steps = 1
		}
		for j := 1; j <= steps; j++ {
			a := float64(start) + float64(sweep)*float64(j)/float64(steps)
			path.LineTo(cx+float32(math.Cos(a))*radius, cy+float32(math.Sin(a))*radius)
		}
		path.Close()
		color := ui.Hex(piePalette[i%len(piePalette)])
		p.FillPath(&path, color)
		p.StrokePath(&path, 1.2, t.Background)
		start += sweep
	}

	lx := cx + radius + 52
	ly := r.Y + titleH + 18
	for i, s := range chart.Slices {
		if ly+22 > r.Y+r.H-8 {
			break
		}
		color := ui.Hex(piePalette[i%len(piePalette)])
		p.Fill(ui.Rect{X: lx, Y: ly + 4, W: 10, H: 10}, color, 2)
		sp := ui.Span{Text: s.Label, Size: 11.5, Color: t.Text}
		p.RichText(lx+16, ly, 150, sp)
		pct := ui.Span{Text: fmt.Sprintf("%.0f%%", s.Value/total*100), Size: 11, Color: t.TextMuted}
		p.RichText(lx+180, ly, 60, pct)
		ly += 24
	}
}
