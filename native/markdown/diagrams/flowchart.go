package diagrams

import "strings"

// DiagramRect is a backend-neutral rectangle in diagram coordinates.
type DiagramRect struct{ X, Y, W, H float32 }

// FlowNode is one labeled flowchart node with its computed rank.
type FlowNode struct {
	ID, Label string
	Level     int
	Rect      DiagramRect
}

// FlowEdge connects node indices in a Flowchart.
type FlowEdge struct {
	From, To int
	Label    string
	Dashed   bool
}

// Flowchart is the parsed graph and rank layout policy for Mermaid flowcharts.
type Flowchart struct {
	Direction string
	Nodes     []FlowNode
	Edges     []FlowEdge
}

// FlowchartOptions injects the localized initial-state label.
type FlowchartOptions struct{ StartLabel string }

// ParseFlowchart parses the supported graph/flowchart subset.
func ParseFlowchart(source string, opts FlowchartOptions) (*Flowchart, bool) {
	chart := &Flowchart{Direction: "TD"}
	byID := make(map[string]int)
	node := func(spec string) int {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			return -1
		}
		id, label := spec, spec
		for _, pair := range [][2]byte{{'[', ']'}, {'(', ')'}, {'{', '}'}} {
			if at := strings.IndexByte(spec, pair[0]); at > 0 {
				end := strings.LastIndexByte(spec, pair[1])
				if end > at+1 {
					id, label = strings.TrimSpace(spec[:at]), strings.TrimSpace(spec[at+1:end])
					break
				}
			}
		}
		label = strings.Trim(label, `"'`)
		if id == "[*]" && opts.StartLabel != "" {
			label = opts.StartLabel
		}
		if i, ok := byID[id]; ok {
			if label != "" && label != id {
				chart.Nodes[i].Label = label
			}
			return i
		}
		byID[id] = len(chart.Nodes)
		chart.Nodes = append(chart.Nodes, FlowNode{ID: id, Label: label})
		return len(chart.Nodes) - 1
	}
	for _, raw := range strings.Split(source, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ";"))
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		low := strings.ToLower(line)
		matchedHeader := false
		for _, prefix := range []string{"graph ", "flowchart "} {
			if strings.HasPrefix(low, prefix) {
				direction := strings.ToUpper(strings.TrimSpace(line[len(prefix):]))
				if direction == "LR" || direction == "RL" || direction == "TD" || direction == "TB" || direction == "BT" {
					chart.Direction = direction
				}
				matchedHeader = true
				break
			}
		}
		if matchedHeader {
			continue
		}
		if strings.HasPrefix(low, "subgraph") {
			return nil, false
		}
		skip := false
		for _, prefix := range []string{"end", "classdef", "class ", "style ", "click ", "linkstyle"} {
			if strings.HasPrefix(low, prefix) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		arrow, at := "", -1
		for _, candidate := range []string{"-.->", "==>", "-->", "---"} {
			if i := strings.Index(line, candidate); i >= 0 && (at < 0 || i < at) {
				arrow, at = candidate, i
			}
		}
		if at < 0 {
			node(line)
			continue
		}
		left := strings.TrimSpace(line[:at])
		right := strings.TrimSpace(line[at+len(arrow):])
		label := ""
		if strings.HasPrefix(right, "|") {
			if end := strings.Index(right[1:], "|"); end >= 0 {
				label = strings.TrimSpace(right[1 : end+1])
				right = strings.TrimSpace(right[end+2:])
			}
		}
		from := node(left)
		if from < 0 {
			continue
		}
		for _, part := range strings.Split(right, "&") {
			to := node(part)
			if to < 0 {
				continue
			}
			chart.Edges = append(chart.Edges, FlowEdge{From: from, To: to, Label: label, Dashed: arrow == "-.->"})
		}
	}
	if len(chart.Nodes) == 0 || len(chart.Edges) == 0 {
		return nil, false
	}
	chart.assignLevels()
	return chart, true
}

// Width returns the minimum width the graph needs for its nodes and labels.
func (g *Flowchart) Width() float32 {
	rows, maxLevel := g.rows()
	const margin = 42
	if g.horizontal() {
		total, prev := float32(32), -1
		for level := 0; level <= maxLevel; level++ {
			colWidth := float32(0)
			for _, i := range rows[level] {
				colWidth = maxF(colWidth, g.nodeWidth(g.Nodes[i]))
			}
			if colWidth == 0 {
				continue
			}
			if prev >= 0 {
				total += g.rankGap(prev)
			}
			total += colWidth
			prev = level
		}
		return total + margin
	}
	widest := float32(0)
	for level := 0; level <= maxLevel; level++ {
		total := float32(0)
		for i, index := range rows[level] {
			total += g.nodeWidth(g.Nodes[index])
			if i > 0 {
				total += 24
			}
		}
		widest = maxF(widest, total)
	}
	return widest + 24 + margin
}

// Height returns the minimum height the graph needs for its ranks.
func (g *Flowchart) Height() float32 {
	_, maxLevel := g.rows()
	if g.horizontal() {
		perRank := make(map[int]int)
		tallest := 0
		for _, node := range g.Nodes {
			perRank[node.Level]++
			tallest = max(tallest, perRank[node.Level])
		}
		return float32(tallest)*52 + 40
	}
	return float32(maxLevel+1)*88 + 20
}

// Layout assigns node rectangles without depending on UI geometry types.
func (g *Flowchart) Layout(width, height float32) []FlowNode {
	rows, maxLevel := g.rows()
	out := append([]FlowNode(nil), g.Nodes...)
	if g.horizontal() {
		x := float32(16)
		prev := -1
		for level := 0; level <= maxLevel; level++ {
			col := rows[level]
			if len(col) == 0 {
				continue
			}
			if prev >= 0 {
				x += g.rankGap(prev)
			}
			colWidth := float32(0)
			for _, index := range col {
				colWidth = maxF(colWidth, g.nodeWidth(g.Nodes[index]))
			}
			startY := float32(12) + (height-12-float32(len(col))*52)/2
			for i, index := range col {
				w := g.nodeWidth(g.Nodes[index])
				out[index].Rect = DiagramRect{X: x + (colWidth-w)/2, Y: startY + float32(i)*52, W: w, H: 36}
			}
			x += colWidth
			prev = level
		}
		return out
	}
	y := float32(12)
	for level := 0; level <= maxLevel; level++ {
		row := rows[level]
		if len(row) == 0 {
			continue
		}
		total := float32(0)
		for i, index := range row {
			total += g.nodeWidth(g.Nodes[index])
			if i > 0 {
				total += 24
			}
		}
		x := (width - total) / 2
		for _, index := range row {
			w := g.nodeWidth(g.Nodes[index])
			out[index].Rect = DiagramRect{X: x, Y: y, W: w, H: 36}
			x += w + 24
		}
		y += 36 + 52
	}
	return out
}

func (g *Flowchart) rows() ([][]int, int) {
	maxLevel := 0
	for _, node := range g.Nodes {
		if node.Level > maxLevel {
			maxLevel = node.Level
		}
	}
	rows := make([][]int, maxLevel+1)
	for i, node := range g.Nodes {
		rows[node.Level] = append(rows[node.Level], i)
	}
	return rows, maxLevel
}

func (g *Flowchart) assignLevels() {
	for i := range g.Nodes {
		g.Nodes[i].Level = -1
	}
	indeg := make([]int, len(g.Nodes))
	out := make([][]int, len(g.Nodes))
	for _, edge := range g.Edges {
		indeg[edge.To]++
		out[edge.From] = append(out[edge.From], edge.To)
	}
	var queue []int
	for i := range g.Nodes {
		if indeg[i] == 0 {
			g.Nodes[i].Level = 0
			queue = append(queue, i)
		}
	}
	if len(queue) == 0 && len(g.Nodes) > 0 {
		g.Nodes[0].Level = 0
		queue = append(queue, 0)
	}
	for len(queue) > 0 {
		from := queue[0]
		queue = queue[1:]
		for _, to := range out[from] {
			if g.Nodes[to].Level < 0 {
				g.Nodes[to].Level = g.Nodes[from].Level + 1
				queue = append(queue, to)
			}
		}
	}
	for i := range g.Nodes {
		if g.Nodes[i].Level < 0 {
			g.Nodes[i].Level = 0
		}
	}
}

func (g *Flowchart) horizontal() bool { return g.Direction == "LR" || g.Direction == "RL" }

func (g *Flowchart) nodeWidth(node FlowNode) float32 {
	w := float32(len([]rune(node.Label)))*7.5 + 28
	return min(max(w, 80), 220)
}

func (g *Flowchart) rankGap(level int) float32 {
	gap := float32(58)
	for _, edge := range g.Edges {
		if edge.Label == "" || g.Nodes[edge.From].Level != level || g.Nodes[edge.To].Level != level+1 {
			continue
		}
		gap = maxF(gap, edgeLabelWidth(edge.Label)+16)
	}
	return gap
}

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

func maxF(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
