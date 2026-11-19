package markdown

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wheregrassdreams/muse-editor/markdown/diagrams"
	"muse/locales"
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
	if opts.DefaultSection == "" {
		opts.DefaultSection = locales.DiagramTask()
	}
	if opts.StartLabel == "" {
		opts.StartLabel = locales.DiagramStart()
	}
	if opts.EndLabel == "" {
		opts.EndLabel = locales.DiagramEnd()
	}
	if opts.Today.IsZero() {
		opts.Today = time.Now()
	}
	model, ok := diagrams.ParseModel(code, opts)
	if !ok {
		return Diagram{}, false
	}
	return DiagramFromModel(model), true
}

// ParseDiagram reads a Mermaid diagram, in the order the renderer tries
// its kinds, and reports false for one it cannot draw.
func ParseDiagram(code string) (Diagram, bool) {
	model, ok := diagrams.ParseFallbackModel(code, diagrams.Options{
		DefaultSection: locales.DiagramTask(), StartLabel: locales.DiagramStart(),
		EndLabel: locales.DiagramEnd(), Today: time.Now(),
	})
	if !ok {
		return Diagram{}, false
	}
	return DiagramFromModel(model), true
}

// DiagramFromModel creates a MyGo drawing adapter for an already parsed,
// backend-neutral diagram model. Layout dimensions remain owned by the model;
// this adapter only translates it into native drawing operations.
func DiagramFromModel(model diagrams.Model) Diagram {
	switch model.Kind {
	case diagrams.ModelSketch:
		return DiagramFromResult(model.Sketch)
	case diagrams.ModelGantt:
		chart := model.Gantt
		return Diagram{
			width:  chart.Width,
			height: func() float32 { return chart.Height() },
			draw:   func(p *ui.Painter, r ui.Rect, t *ui.Theme) { drawGantt(p, r, t, chart) },
		}
	case diagrams.ModelClass:
		g := adaptClassModel(model.Class)
		return Diagram{width: g.width, height: g.height, draw: g.draw}
	case diagrams.ModelPie:
		chart := model.Pie
		return Diagram{
			width:  func() float32 { return diagrams.PieWidth },
			height: func() float32 { return 286 },
			draw:   func(p *ui.Painter, r ui.Rect, t *ui.Theme) { drawPie(p, r, t, chart) },
		}
	case diagrams.ModelState:
		g := adaptStateModel(model.State)
		return Diagram{width: g.width, height: g.height, draw: g.draw}
	case diagrams.ModelSequence:
		graph := model.Sequence
		return Diagram{
			width:  graph.Width,
			height: func() float32 { return graph.Height() },
			draw:   func(p *ui.Painter, r ui.Rect, t *ui.Theme) { drawSequence(p, r, t, graph) },
		}
	case diagrams.ModelFlowchart:
		g := adaptFlowchart(model.Flowchart)
		return Diagram{width: g.width, height: g.height, draw: g.draw}
	}
	return Diagram{}
}
