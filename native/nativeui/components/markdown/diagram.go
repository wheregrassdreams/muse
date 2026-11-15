package markdown

import (
	"time"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/markdown/diagrams"
)

// Diagram is a Mermaid diagram mdoc draws natively: flowcharts, sequence,
// state, pie, class and Gantt subsets.
type Diagram struct {
	width  func() float32
	height func() float32
	draw   func(p *ui.Painter, r ui.Rect, t *ui.Theme)
}

// Width returns the least width the diagram draws well at: drawn narrower,
// its nodes and labels spill out of the box.
func (d Diagram) Width() float32 { return d.width() }

// Height returns how tall the diagram is at a width.
func (d Diagram) Height(width float32) float32 { return d.height() }

// Draw paints the diagram in r, in the colors of t.
func (d Diagram) Draw(p *ui.Painter, r ui.Rect, t *ui.Theme) { d.draw(p, r, t) }

// ParseDiagramThemed reads a Mermaid diagram and draws it in the colors and
// the text widths of the page it is in. sketch draws every Mermaid kind pele
// draws; the older parsers here are the fallback for what it refuses.
func ParseDiagramThemed(code string, opts diagrams.Options) (Diagram, bool) {
	if d, ok := ParseSketchDiagram(code, opts); ok {
		return d, true
	}
	return ParseDiagram(code)
}

// ParseDiagram reads a Mermaid diagram, in the order the renderer tries
// its kinds, and reports false for one it cannot draw.
func ParseDiagram(code string) (Diagram, bool) {
	if chart, ok := diagrams.ParseGantt(code, diagrams.GanttOptions{DefaultSection: locales.DiagramTask(), Today: time.Now()}); ok {
		return Diagram{
			width:  chart.Width,
			height: func() float32 { return chart.Height() },
			draw:   func(p *ui.Painter, r ui.Rect, t *ui.Theme) { drawGantt(p, r, t, chart) },
		}, true
	}
	if g, ok := adaptClass(code); ok {
		return Diagram{width: g.width, height: g.height, draw: g.draw}, true
	}
	if chart, ok := diagrams.ParsePie(code); ok {
		return Diagram{
			width:  func() float32 { return diagrams.PieWidth },
			height: func() float32 { return 286 },
			draw:   func(p *ui.Painter, r ui.Rect, t *ui.Theme) { drawPie(p, r, t, chart) },
		}, true
	}
	if g, ok := adaptStateDiagram(code); ok {
		return Diagram{width: g.width, height: g.height, draw: g.draw}, true
	}
	if graph, ok := diagrams.ParseSequence(code); ok {
		return Diagram{
			width:  graph.Width,
			height: func() float32 { return graph.Height() },
			draw:   func(p *ui.Painter, r ui.Rect, t *ui.Theme) { drawSequence(p, r, t, graph) },
		}, true
	}
	if g, ok := adaptFlowchartSource(code); ok {
		return Diagram{width: g.width, height: g.height, draw: g.draw}, true
	}
	return Diagram{}, false
}
