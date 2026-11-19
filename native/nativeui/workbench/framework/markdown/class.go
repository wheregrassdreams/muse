package markdown

import (
	"github.com/egoist/mygo/ui"
	"github.com/wheregrassdreams/muse-editor/markdown/diagrams"
)

type classGraph struct {
	model     *diagrams.ClassDiagram
	classes   []*classNode
	relations []classRel
}

type classNode struct {
	name    string
	members []string
	rect    ui.Rect
}

type classRel struct {
	from, to *classNode
	kind     string
	label    string
}

func adaptClass(source string) (*classGraph, bool) {
	model, ok := diagrams.ParseClass(source)
	if !ok {
		return nil, false
	}
	return adaptClassModel(model), true
}

func adaptClassModel(model *diagrams.ClassDiagram) *classGraph {
	graph := &classGraph{model: model, classes: make([]*classNode, len(model.Classes))}
	for i, class := range model.Classes {
		graph.classes[i] = &classNode{name: class.Name, members: append([]string(nil), class.Members...)}
	}
	for _, relation := range model.Relations {
		if relation.From < 0 || relation.From >= len(graph.classes) || relation.To < 0 || relation.To >= len(graph.classes) {
			continue
		}
		graph.relations = append(graph.relations, classRel{from: graph.classes[relation.From], to: graph.classes[relation.To], kind: relation.Kind, label: relation.Label})
	}
	return graph
}

func (g *classGraph) width() float32  { return g.model.Width() }
func (g *classGraph) height() float32 { return g.model.Height() }

func renderClass(c *ui.Context, g *classGraph, _ rstyle) {
	t := c.Theme()
	ui.Box(c).FillWidth().Height(g.height()).Draw(func(p *ui.Painter, r ui.Rect) { g.draw(p, r, t) })
}

func (g *classGraph) draw(p *ui.Painter, r ui.Rect, t *ui.Theme) {
	layout := g.model.Layout(r.W)
	for i, class := range g.classes {
		if i >= len(layout) {
			continue
		}
		box := layout[i].Rect
		class.rect = ui.Rect{X: r.X + box.X, Y: r.Y + box.Y, W: box.W, H: box.H}
	}
	for _, relation := range g.relations {
		drawClassRel(p, t, relation)
	}
	for _, class := range g.classes {
		box := class.rect
		p.Fill(box, t.Surface, 6)
		p.Stroke(box, t.Border, 6, 1)
		name := ui.Span{Text: class.name, Size: 12.5, Weight: 650, Color: t.Text}
		nw, _ := p.MeasureText(box.W-16, name)
		p.RichText(box.X+(box.W-nw)/2, box.Y+7, box.W-16, name)
		if len(class.members) == 0 {
			continue
		}
		p.Line(box.X, box.Y+30, box.X+box.W, box.Y+30, 1, t.Border)
		y := box.Y + 35
		for _, member := range class.members {
			sp := ui.Span{Text: member, Size: 10.5, Color: t.TextMuted}
			p.RichText(box.X+9, y, box.W-18, sp)
			y += 17
		}
	}
}

func drawClassRel(p *ui.Painter, t *ui.Theme, rel classRel) {
	a, b := rel.from.rect, rel.to.rect
	x1, y1 := a.X+a.W/2, a.Y+a.H
	x2, y2 := b.X+b.W/2, b.Y
	// A child above its parent (cycle) is routed around the right side.
	if y2 < y1 {
		x1 = a.X + a.W
		y1 = a.Y + a.H/2
		x2 = b.X + b.W
		y2 = b.Y + b.H/2
		mid := maxF(x1, x2) + 26
		p.Line(x1, y1, mid, y1, 1.3, t.TextMuted)
		p.Line(mid, y1, mid, y2, 1.3, t.TextMuted)
		p.Line(mid, y2, x2, y2, 1.3, t.TextMuted)
		return
	}
	dashed := rel.kind == "depend" || rel.kind == "realize"
	if dashed {
		drawDashedLine(p, x1, y1, x2, y2, t.TextMuted)
	} else {
		p.Line(x1, y1, x2, y2, 1.3, t.TextMuted)
	}
	switch rel.kind {
	case "inherit", "realize":
		// Hollow triangle at the parent end.
		path := ui.Path{}
		path.MoveTo(x1, y1)
		path.LineTo(x1-6, y1+11)
		path.LineTo(x1+6, y1+11)
		path.Close()
		p.FillPath(&path, t.Background)
		p.StrokePath(&path, 1.2, t.TextMuted)
	case "compose", "aggregate":
		// Diamond at the owner end.
		path := ui.Path{}
		path.MoveTo(x1, y1)
		path.LineTo(x1-6, y1-11)
		path.LineTo(x1, y1-22)
		path.LineTo(x1+6, y1-11)
		path.Close()
		if rel.kind == "compose" {
			p.FillPath(&path, t.TextMuted)
		} else {
			p.FillPath(&path, t.Background)
			p.StrokePath(&path, 1.2, t.TextMuted)
		}
	default:
		p.Line(x2, y2, x2-5, y2+8, 1.2, t.TextMuted)
		p.Line(x2, y2, x2+5, y2+8, 1.2, t.TextMuted)
	}
	if rel.label != "" {
		sp := ui.Span{Text: rel.label, Size: 10, Color: t.TextMuted}
		tw, _ := p.MeasureText(0, sp)
		p.RichText((x1+x2)/2-tw/2, (y1+y2)/2-12, 0, sp)
	}
}

func drawDashedLine(p *ui.Painter, x1, y1, x2, y2 float32, c ui.Color) {
	dx, dy := x2-x1, y2-y1
	length := float32(1)
	if dx != 0 || dy != 0 {
		length = float32(1)
	}
	// Simple dash walk, good enough for the mostly vertical class edges.
	steps := int((absF(dx) + absF(dy)) / 8)
	if steps < 1 {
		steps = 1
	}
	for i := 0; i < steps; i += 2 {
		t0 := float32(i) / float32(steps)
		t1 := float32(i+1) / float32(steps)
		p.Line(x1+dx*t0, y1+dy*t0, x1+dx*t1, y1+dy*t1, 1.2, c)
	}
	_ = length
}

func absF(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
