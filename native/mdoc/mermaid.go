package mdoc

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// mermaidGraph is the subset of Mermaid flowcharts Muse draws natively:
// graph/flowchart TD or LR, labelled nodes and labelled/dashed arrows.
// Sequence/class/state/Gantt diagrams are left as code.
type mermaidGraph struct {
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

func parseMermaid(src string) (*mermaidGraph, bool) {
	g := &mermaidGraph{dir: "TD", byID: map[string]*mermaidNode{}}
	lines := strings.Split(src, "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ";"))
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		low := strings.ToLower(line)
		for _, p := range []string{"graph ", "flowchart "} {
			if strings.HasPrefix(low, p) {
				dir := strings.ToUpper(strings.TrimSpace(line[len(p):]))
				if dir == "LR" || dir == "RL" || dir == "TD" || dir == "TB" || dir == "BT" {
					g.dir = dir
				}
				line = ""
				break
			}
		}
		if line == "" {
			continue
		}
		for _, skip := range []string{"subgraph", "end", "classdef", "class ", "style ", "click ", "linkstyle"} {
			if strings.HasPrefix(low, skip) {
				line = ""
				break
			}
		}
		if line == "" {
			continue
		}
		// Find the first edge arrow.
		arrow, at := "", -1
		for _, a := range []string{"-.->", "==>", "-->", "---"} {
			if i := strings.Index(line, a); i >= 0 && (at < 0 || i < at) {
				arrow, at = a, i
			}
		}
		if at < 0 {
			g.node(line)
			continue
		}
		left := strings.TrimSpace(line[:at])
		right := strings.TrimSpace(line[at+len(arrow):])
		label := ""
		if strings.HasPrefix(right, "|") {
			if end := strings.Index(right[1:], "|"); end >= 0 {
				label = strings.TrimSpace(right[1 : 1+end])
				right = strings.TrimSpace(right[2+end:])
			}
		}
		from := g.node(left)
		if from == nil {
			continue
		}
		for _, part := range strings.Split(right, "&") {
			to := g.node(strings.TrimSpace(part))
			if to == nil {
				continue
			}
			g.edges = append(g.edges, mermaidEdge{from: from, to: to, label: label, dashed: arrow == "-.->"})
		}
	}
	if len(g.nodes) == 0 || len(g.edges) == 0 {
		return nil, false
	}
	g.levels()
	return g, true
}

func (g *mermaidGraph) node(spec string) *mermaidNode {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	id, label := spec, spec
	for i, open := range map[byte]byte{'[': ']', '(': ')', '{': '}'} {
		if j := strings.IndexByte(spec, i); j > 0 {
			close := strings.LastIndexByte(spec, open)
			if close > j+1 {
				id = strings.TrimSpace(spec[:j])
				label = strings.TrimSpace(spec[j+1 : close])
				break
			}
		}
	}
	label = strings.Trim(label, `"'`)
	if id == "[*]" {
		label = "开始"
	}
	if n, ok := g.byID[id]; ok {
		if label != "" && label != id {
			n.label = label
		}
		return n
	}
	n := &mermaidNode{id: id, label: label}
	g.byID[id] = n
	g.nodes = append(g.nodes, n)
	return n
}

func (g *mermaidGraph) levels() {
	for _, n := range g.nodes {
		n.level = -1
	}
	indeg := map[*mermaidNode]int{}
	out := map[*mermaidNode][]*mermaidNode{}
	for _, e := range g.edges {
		indeg[e.to]++
		out[e.from] = append(out[e.from], e.to)
	}
	var queue []*mermaidNode
	for _, n := range g.nodes {
		if indeg[n] == 0 {
			n.level = 0
			queue = append(queue, n)
		}
	}
	if len(queue) == 0 && len(g.nodes) > 0 {
		g.nodes[0].level = 0
		queue = append(queue, g.nodes[0])
	}
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, v := range out[u] {
			if v.level < 0 {
				v.level = u.level + 1
				queue = append(queue, v)
			}
		}
	}
	for _, n := range g.nodes {
		if n.level < 0 {
			n.level = 0
		}
	}
}

func (g *mermaidGraph) horizontal() bool {
	return g.dir == "LR" || g.dir == "RL"
}

func (g *mermaidGraph) nodeWidth(n *mermaidNode) float32 {
	w := float32(len([]rune(n.label)))*7.5 + 28
	if w < 80 {
		w = 80
	}
	if w > 220 {
		w = 220
	}
	return w
}

func (g *mermaidGraph) height() float32 {
	maxLevel := 0
	for _, n := range g.nodes {
		if n.level > maxLevel {
			maxLevel = n.level
		}
	}
	if g.horizontal() {
		// Sideways, each rank is a column of nodes stacked 52 DIPs apart:
		// the tallest one sets the height, with room for a back edge
		// routed below.
		perRank := map[int]int{}
		tallest := 0
		for _, n := range g.nodes {
			perRank[n.level]++
			tallest = max(tallest, perRank[n.level])
		}
		return float32(tallest)*52 + 40
	}
	return float32(maxLevel+1)*88 + 20
}

// edgeLabelWidth estimates how wide an edge's label draws at the labels'
// font: wide runes, as CJK's, take the font's size, narrow ones about six
// tenths of it. The layout uses it to keep the labels clear of the nodes.
func edgeLabelWidth(label string) float32 {
	w := float32(0)
	for _, r := range label {
		if r >= 0x2E80 {
			w += 10.5
		} else {
			w += 6
		}
	}
	return w
}

// rankGap returns the room between the columns of level l and the next:
// enough for the widest label crossing it, and never less than the plain
// gap.
func (g *mermaidGraph) rankGap(l int) float32 {
	gap := float32(58)
	for _, e := range g.edges {
		if e.label == "" || e.from.level != l || e.to.level != l+1 {
			continue
		}
		gap = maxF(gap, edgeLabelWidth(e.label)+16)
	}
	return gap
}

// width returns the least width the graph draws well at: the widest rank
// of a top-down graph, or a sideways graph's ranks and their gaps, plus
// room for labels and the edges routed outside.
func (g *mermaidGraph) width() float32 {
	rows := map[int][]*mermaidNode{}
	maxLevel := 0
	for _, n := range g.nodes {
		rows[n.level] = append(rows[n.level], n)
		if n.level > maxLevel {
			maxLevel = n.level
		}
	}
	// Back edges route 26 beyond the nodes, and rank margins add 16 a side.
	const margin = 42
	if g.horizontal() {
		total := float32(32)
		prev := -1
		for l := 0; l <= maxLevel; l++ {
			colW := float32(0)
			for _, n := range rows[l] {
				colW = maxF(colW, g.nodeWidth(n))
			}
			if colW == 0 {
				continue
			}
			if prev >= 0 {
				total += g.rankGap(prev)
			}
			total += colW
			prev = l
		}
		return total + margin
	}
	widest := float32(0)
	for l := 0; l <= maxLevel; l++ {
		total := float32(0)
		for i, n := range rows[l] {
			total += g.nodeWidth(n)
			if i > 0 {
				total += 24
			}
		}
		widest = maxF(widest, total)
	}
	return widest + 2*12 + margin
}

func renderMermaid(c *ui.Context, g *mermaidGraph, s rstyle) {
	t := c.Theme()
	ui.Box(c).FillWidth().Height(g.height()).Draw(func(p *ui.Painter, r ui.Rect) { g.draw(p, r, t) })
}

// draw paints the diagram in r.
func (g *mermaidGraph) draw(p *ui.Painter, r ui.Rect, t *ui.Theme) {
	maxLevel := 0
	for _, n := range g.nodes {
		if n.level > maxLevel {
			maxLevel = n.level
		}
	}
	rows := make([][]*mermaidNode, maxLevel+1)
	for _, n := range g.nodes {
		rows[n.level] = append(rows[n.level], n)
	}
	g.layout(r)
	for _, e := range g.edges {
		g.drawEdge(p, e, t)
	}
	for _, n := range g.nodes {
		rect := n.rect
		p.Fill(rect, t.Surface, 7)
		p.Stroke(rect, t.Border, 7, 1)
		sp := ui.Span{Text: n.label, Size: 12.5, Color: t.Text}
		tw, th := p.MeasureText(rect.W-16, sp)
		p.RichText(rect.X+(rect.W-tw)/2, rect.Y+(rect.H-th)/2, rect.W-16, sp)
	}
}

func (g *mermaidGraph) layout(r ui.Rect) {
	rows := map[int][]*mermaidNode{}
	maxLevel := 0
	for _, n := range g.nodes {
		rows[n.level] = append(rows[n.level], n)
		if n.level > maxLevel {
			maxLevel = n.level
		}
	}
	if g.horizontal() {
		x := r.X + 16
		prev := -1
		for l := 0; l <= maxLevel; l++ {
			col := rows[l]
			if len(col) == 0 {
				continue
			}
			if prev >= 0 {
				x += g.rankGap(prev)
			}
			colW := float32(0)
			for _, n := range col {
				if w := g.nodeWidth(n); w > colW {
					colW = w
				}
			}
			startY := r.Y + 12 + (r.H-12-float32(len(col))*52)/2
			for i, n := range col {
				w := g.nodeWidth(n)
				n.rect = ui.Rect{X: x + (colW-w)/2, Y: startY + float32(i)*52, W: w, H: 36}
			}
			x += colW
			prev = l
		}
		return
	}
	rowGap := float32(52)
	y := r.Y + 12
	for l := 0; l <= maxLevel; l++ {
		row := rows[l]
		if len(row) == 0 {
			continue
		}
		total := float32(0)
		for i, n := range row {
			total += g.nodeWidth(n)
			if i > 0 {
				total += 24
			}
		}
		x := r.X + (r.W-total)/2
		for _, n := range row {
			w := g.nodeWidth(n)
			n.rect = ui.Rect{X: x, Y: y, W: w, H: 36}
			x += w + 24
		}
		y += 36 + rowGap
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
