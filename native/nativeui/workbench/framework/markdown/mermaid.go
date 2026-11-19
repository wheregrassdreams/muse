package markdown

import (
	"github.com/egoist/mygo/ui"
	"github.com/wheregrassdreams/muse-editor/markdown/diagrams"
	"muse/locales"
)

// mermaidGraph is the MyGo painter adapter for the backend-neutral flowchart
// model and its computed layout.
type mermaidGraph struct {
	model *diagrams.Flowchart
	dir   string
	nodes []*mermaidNode
	edges []mermaidEdge
	byID  map[string]*mermaidNode
}

type mermaidNode struct {
	id, label string
	level     int
	rect      ui.Rect
}

type mermaidEdge struct {
	from, to *mermaidNode
	label    string
	dashed   bool
}

func adaptFlowchart(model *diagrams.Flowchart) *mermaidGraph {
	graph := &mermaidGraph{model: model, dir: model.Direction, byID: make(map[string]*mermaidNode, len(model.Nodes))}
	for _, node := range model.Nodes {
		adapted := &mermaidNode{id: node.ID, label: node.Label, level: node.Level}
		graph.nodes = append(graph.nodes, adapted)
		graph.byID[node.ID] = adapted
	}
	for _, edge := range model.Edges {
		if edge.From < 0 || edge.From >= len(graph.nodes) || edge.To < 0 || edge.To >= len(graph.nodes) {
			continue
		}
		graph.edges = append(graph.edges, mermaidEdge{from: graph.nodes[edge.From], to: graph.nodes[edge.To], label: edge.Label, dashed: edge.Dashed})
	}
	return graph
}

func adaptFlowchartSource(source string) (*mermaidGraph, bool) {
	model, ok := diagrams.ParseFlowchart(source, diagrams.FlowchartOptions{StartLabel: locales.DiagramStart()})
	if !ok {
		return nil, false
	}
	return adaptFlowchart(model), true
}

func (g *mermaidGraph) horizontal() bool {
	return g.model.Direction == "LR" || g.model.Direction == "RL"
}
func (g *mermaidGraph) width() float32  { return g.model.Width() }
func (g *mermaidGraph) height() float32 { return g.model.Height() }

func renderMermaid(c *ui.Context, g *mermaidGraph, _ rstyle) {
	t := c.Theme()
	ui.Box(c).FillWidth().Height(g.height()).Draw(func(p *ui.Painter, r ui.Rect) { g.draw(p, r, t) })
}

func (g *mermaidGraph) draw(p *ui.Painter, r ui.Rect, t *ui.Theme) {
	layout := g.model.Layout(r.W, r.H)
	for i, node := range g.nodes {
		if i >= len(layout) {
			continue
		}
		box := layout[i].Rect
		node.rect = ui.Rect{X: r.X + box.X, Y: r.Y + box.Y, W: box.W, H: box.H}
	}
	for _, edge := range g.edges {
		g.drawEdge(p, edge, t)
	}
	for _, node := range g.nodes {
		rect := node.rect
		p.Fill(rect, t.Surface, 7)
		p.Stroke(rect, t.Border, 7, 1)
		sp := ui.Span{Text: node.label, Size: 12.5, Color: t.Text}
		tw, th := p.MeasureText(rect.W-16, sp)
		p.RichText(rect.X+(rect.W-tw)/2, rect.Y+(rect.H-th)/2, rect.W-16, sp)
	}
}

func (g *mermaidGraph) drawEdge(p *ui.Painter, e mermaidEdge, t *ui.Theme) {
	a, b := e.from.rect, e.to.rect
	color := t.TextMuted
	// Back edges take a route outside the column so they do not overlap the
	// forward edge and its label.
	if g.horizontal() {
		back := (g.dir == "LR" && b.X < a.X) || (g.dir == "RL" && b.X > a.X)
		if back {
			x0 := a.X + a.W/2
			y0 := a.Y + a.H
			x2 := b.X + b.W/2
			y2 := b.Y + b.H
			mid := maxF(y0, y2) + 26
			p.Line(x0, y0, x0, mid, 1.4, color)
			p.Line(x0, mid, x2, mid, 1.4, color)
			p.Line(x2, mid, x2, y2, 1.4, color)
			p.Line(x2, y2, x2-4, y2+6, 1.4, color)
			p.Line(x2, y2, x2+4, y2+6, 1.4, color)
			if e.label != "" {
				sp := ui.Span{Text: e.label, Size: 10.5, Color: t.TextMuted}
				tw, _ := p.MeasureText(0, sp)
				p.RichText((x0+x2)/2-tw/2, mid+4, 0, sp)
			}
			return
		}
	} else {
		back := (g.dir == "TD" || g.dir == "TB") && b.Y < a.Y
		if back {
			x0 := a.X + a.W
			y0 := a.Y + a.H/2
			x2 := b.X + b.W
			y2 := b.Y + b.H/2
			mid := maxF(x0, x2) + 26
			p.Line(x0, y0, mid, y0, 1.4, color)
			p.Line(mid, y0, mid, y2, 1.4, color)
			p.Line(mid, y2, x2, y2, 1.4, color)
			p.Line(x2, y2, x2+6, y2-4, 1.4, color)
			p.Line(x2, y2, x2+6, y2+4, 1.4, color)
			if e.label != "" {
				sp := ui.Span{Text: e.label, Size: 10.5, Color: t.TextMuted}
				p.RichText(mid+4, (y0+y2)/2, 0, sp)
			}
			return
		}
	}

	var x1, y1, x2, y2 float32
	if g.horizontal() {
		if g.dir == "RL" {
			x1, y1, x2, y2 = a.X, a.Y+a.H/2, b.X+b.W, b.Y+b.H/2
		} else {
			x1, y1, x2, y2 = a.X+a.W, a.Y+a.H/2, b.X, b.Y+b.H/2
		}
	} else if g.dir == "BT" {
		x1, y1, x2, y2 = a.X+a.W/2, a.Y, b.X+b.W/2, b.Y+b.H
	} else {
		x1, y1, x2, y2 = a.X+a.W/2, a.Y+a.H, b.X+b.W/2, b.Y
	}
	// A top-down edge that has to move sideways goes across a bus between
	// the two ranks instead of cutting the diagonal: the bus is where the
	// label sits, and the label covers the line it rides.
	bus := g.busY(a, b)
	if bus > 0 && !g.horizontal() && x1 != x2 {
		p.Line(x1, y1, x1, bus, 1.4, color)
		p.Line(x1, bus, x2, bus, 1.4, color)
		p.Line(x2, bus, x2, y2, 1.4, color)
	} else {
		p.Line(x1, y1, x2, y2, 1.4, color)
	}
	// Arrowhead.
	if g.horizontal() {
		p.Line(x2, y2, x2-6, y2-4, 1.4, color)
		p.Line(x2, y2, x2-6, y2+4, 1.4, color)
	} else if g.dir == "BT" {
		p.Line(x2, y2, x2-4, y2-6, 1.4, color)
		p.Line(x2, y2, x2+4, y2-6, 1.4, color)
	} else {
		p.Line(x2, y2, x2-4, y2+6, 1.4, color)
		p.Line(x2, y2, x2+4, y2+6, 1.4, color)
	}
	if e.label != "" {
		sp := ui.Span{Text: e.label, Size: 10.5, Color: t.TextMuted}
		tw, th := p.MeasureText(0, sp)
		lx, ly := (x1+x2)/2-tw/2, (y1+y2)/2-th/2-4
		if bus > 0 && !g.horizontal() {
			// On the bus, past the corner: the plate covers the line.
			ly = bus - th/2
			if x2 > x1 {
				lx = x1 + (x2-x1)*0.5 - tw/2
			}
		}
		p.Fill(ui.Rect{X: lx - 3, Y: ly - 1, W: tw + 6, H: th + 2}, t.Background, 3)
		p.RichText(lx, ly, 0, sp)
	}
}

// busY returns the y of the line an edge crosses sideways at, for a graph
// that runs top-down: the middle of the room between the two ranks, or 0
// when the edge does not need one.
func (g *mermaidGraph) busY(a, b ui.Rect) float32 {
	if g.horizontal() {
		return 0
	}
	if g.dir == "BT" {
		return (a.Y - (b.Y + b.H)) / 2
	}
	if b.Y < a.Y {
		return 0 // a back edge has its own route
	}
	return a.Y + a.H + (b.Y-(a.Y+a.H))/2
}

func maxF(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
