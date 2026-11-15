package diagrams

import "strings"

// Class is a class box and its displayed members.
type Class struct {
	Name    string
	Members []string
	Level   int
	Width   float32
	Height  float32
	Rect    DiagramRect
}

// ClassRelation connects classes with a Mermaid relationship kind.
type ClassRelation struct {
	From, To int
	Kind     string
	Label    string
}

// ClassDiagram is a parsed class model and rank layout policy.
type ClassDiagram struct {
	Classes    []Class
	Relations  []ClassRelation
	rowHeights []float32
}

// ParseClass parses Muse's supported Mermaid class diagram subset.
func ParseClass(source string) (*ClassDiagram, bool) {
	lines := strings.Split(source, "\n")
	if len(lines) == 0 || !strings.EqualFold(strings.TrimSpace(lines[0]), "classdiagram") {
		return nil, false
	}
	diagram := &ClassDiagram{}
	byName := map[string]int{}
	class := func(name string) int {
		name = strings.TrimSpace(name)
		if name == "" || strings.Contains(name, " ") && strings.Contains(name, `"`) {
			return -1
		}
		if i, ok := byName[name]; ok {
			return i
		}
		byName[name] = len(diagram.Classes)
		diagram.Classes = append(diagram.Classes, Class{Name: name})
		return len(diagram.Classes) - 1
	}
	inBody := false
	for _, raw := range lines[1:] {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if inBody {
			if line == "}" {
				inBody = false
				continue
			}
			if len(diagram.Classes) > 0 {
				i := len(diagram.Classes) - 1
				diagram.Classes[i].Members = append(diagram.Classes[i].Members, line)
			}
			continue
		}
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "direction ") || strings.HasPrefix(low, "namespace ") ||
			strings.HasPrefix(low, "note ") || strings.HasPrefix(low, "style ") ||
			strings.HasPrefix(low, "classdef") || strings.HasPrefix(low, "cssclass") ||
			strings.HasPrefix(low, "<<") {
			continue
		}
		if strings.HasPrefix(low, "class ") {
			rest := strings.TrimSpace(line[len("class "):])
			if strings.HasSuffix(rest, "{") {
				rest = strings.TrimSpace(strings.TrimSuffix(rest, "{"))
				inBody = true
			}
			class(rest)
			continue
		}
		if arrow, at := findClassArrow(line); at >= 0 {
			left := strings.TrimSpace(line[:at])
			right := strings.TrimSpace(line[at+len(arrow):])
			label := ""
			if colon := strings.Index(right, ":"); colon >= 0 {
				label = strings.TrimSpace(right[colon+1:])
				right = strings.TrimSpace(right[:colon])
			}
			from, to := class(cleanClassName(left)), class(cleanClassName(right))
			if from < 0 || to < 0 || from == to {
				continue
			}
			kind, reverse := "assoc", false
			switch arrow {
			case "<|--":
				kind = "inherit"
			case "<|..":
				kind = "realize"
			case "*--":
				kind = "compose"
			case "o--":
				kind = "aggregate"
			case "<--":
				reverse = true
			case "..>":
				kind = "depend"
			case "<..":
				kind, reverse = "depend", true
			case "..|>":
				kind = "realize"
			}
			if reverse {
				from, to = to, from
			}
			diagram.Relations = append(diagram.Relations, ClassRelation{From: from, To: to, Kind: kind, Label: label})
			continue
		}
		if colon := strings.Index(line, ":"); colon >= 0 {
			if i := class(cleanClassName(strings.TrimSpace(line[:colon]))); i >= 0 {
				diagram.Classes[i].Members = append(diagram.Classes[i].Members, strings.TrimSpace(line[colon+1:]))
			}
		}
	}
	if len(diagram.Classes) == 0 {
		return nil, false
	}
	diagram.assignLevels()
	diagram.assignSizes()
	return diagram, true
}

// Width returns the minimum width for the widest class rank and routed cycles.
func (g *ClassDiagram) Width() float32 {
	widest := float32(0)
	for level := range g.rowHeights {
		total, count := float32(0), 0
		for _, class := range g.Classes {
			if class.Level != level {
				continue
			}
			if count > 0 {
				total += 30
			}
			total += class.Width
			count++
		}
		widest = maxF(widest, total)
	}
	return widest + 84
}

// Height returns the total height of class ranks.
func (g *ClassDiagram) Height() float32 {
	h := float32(20)
	for _, rowHeight := range g.rowHeights {
		h += rowHeight + 54
	}
	return h
}

// Layout computes class boxes at the given width.
func (g *ClassDiagram) Layout(width float32) []Class {
	out := append([]Class(nil), g.Classes...)
	y := float32(16)
	for level, rowHeight := range g.rowHeights {
		var row []int
		total := float32(0)
		for i, class := range g.Classes {
			if class.Level == level {
				if len(row) > 0 {
					total += 30
				}
				total += class.Width
				row = append(row, i)
			}
		}
		x := (width - total) / 2
		for _, i := range row {
			class := &out[i]
			class.Rect = DiagramRect{X: x, Y: y + (rowHeight-class.Height)/2, W: class.Width, H: class.Height}
			x += class.Width + 30
		}
		y += rowHeight + 54
	}
	return out
}

// Segment is one backend-neutral line in a relationship route.
type Segment struct{ X1, Y1, X2, Y2 float32 }

// ClassRelationLayout describes the geometry needed to paint one relation.
type ClassRelationLayout struct {
	Kind     string
	Label    string
	Dashed   bool
	Segments []Segment
	Start    Point
	End      Point
	LabelX   float32
	LabelY   float32
	BackEdge bool
}

// Point is a location in diagram coordinates.
type Point struct{ X, Y float32 }

// RouteRelations computes relation paths and labels from laid out class boxes.
func (g *ClassDiagram) RouteRelations(classes []Class) []ClassRelationLayout {
	if len(classes) != len(g.Classes) {
		return nil
	}
	out := make([]ClassRelationLayout, 0, len(g.Relations))
	for _, relation := range g.Relations {
		if relation.From < 0 || relation.From >= len(classes) || relation.To < 0 || relation.To >= len(classes) {
			continue
		}
		a, b := classes[relation.From].Rect, classes[relation.To].Rect
		x1, y1 := a.X+a.W/2, a.Y+a.H
		x2, y2 := b.X+b.W/2, b.Y
		route := ClassRelationLayout{Kind: relation.Kind, Label: relation.Label, Dashed: relation.Kind == "depend" || relation.Kind == "realize", Start: Point{x1, y1}, End: Point{x2, y2}}
		if y2 < y1 {
			x1, y1 = a.X+a.W, a.Y+a.H/2
			x2, y2 = b.X+b.W, b.Y+b.H/2
			mid := maxF(x1, x2) + 26
			route.Segments = []Segment{{x1, y1, mid, y1}, {mid, y1, mid, y2}, {mid, y2, x2, y2}}
			route.Start, route.End, route.BackEdge = Point{x1, y1}, Point{x2, y2}, true
			out = append(out, route)
			continue
		}
		route.Segments = []Segment{{x1, y1, x2, y2}}
		route.Start, route.End = Point{x1, y1}, Point{x2, y2}
		route.LabelX = (x1 + x2) / 2
		route.LabelY = (y1+y2)/2 - 12
		out = append(out, route)
	}
	return out
}

func (g *ClassDiagram) assignLevels() {
	for i := range g.Classes {
		g.Classes[i].Level = -1
	}
	indeg := make([]int, len(g.Classes))
	out := make([][]int, len(g.Classes))
	for _, rel := range g.Relations {
		indeg[rel.To]++
		out[rel.From] = append(out[rel.From], rel.To)
	}
	var queue []int
	for i := range g.Classes {
		if indeg[i] == 0 {
			g.Classes[i].Level = 0
			queue = append(queue, i)
		}
	}
	if len(queue) == 0 && len(g.Classes) > 0 {
		g.Classes[0].Level = 0
		queue = append(queue, 0)
	}
	for len(queue) > 0 {
		from := queue[0]
		queue = queue[1:]
		for _, to := range out[from] {
			if g.Classes[to].Level < 0 {
				g.Classes[to].Level = g.Classes[from].Level + 1
				queue = append(queue, to)
			}
		}
	}
	for i := range g.Classes {
		if g.Classes[i].Level < 0 {
			g.Classes[i].Level = 0
		}
	}
}

func (g *ClassDiagram) assignSizes() {
	maxLevel := 0
	for i := range g.Classes {
		class := &g.Classes[i]
		w := float32(len([]rune(class.Name)))*8 + 32
		for _, member := range class.Members {
			if mw := float32(len([]rune(member)))*6.4 + 24; mw > w {
				w = mw
			}
		}
		class.Width = min(max(w, 130), 260)
		class.Height = 30
		if len(class.Members) > 0 {
			class.Height += float32(len(class.Members))*17 + 8
		}
		maxLevel = max(maxLevel, class.Level)
	}
	g.rowHeights = make([]float32, maxLevel+1)
	for _, class := range g.Classes {
		g.rowHeights[class.Level] = maxF(g.rowHeights[class.Level], class.Height)
	}
}

func findClassArrow(s string) (string, int) {
	for _, arrow := range []string{"<|--", "<|..", "..|>", "..>", "<..", "*--", "o--", "-->", "<--", "--", ".."} {
		if i := strings.Index(s, arrow); i >= 0 {
			return arrow, i
		}
	}
	return "", -1
}

func cleanClassName(s string) string {
	s = strings.TrimSpace(s)
	for {
		if strings.HasPrefix(s, `"`) {
			if end := strings.Index(s[1:], `"`); end >= 0 {
				s = strings.TrimSpace(s[end+2:])
				continue
			}
		}
		if strings.HasSuffix(s, `"`) {
			if start := strings.LastIndex(s[:len(s)-1], `"`); start >= 0 {
				s = strings.TrimSpace(s[:start])
				continue
			}
		}
		break
	}
	return strings.TrimSpace(strings.TrimSuffix(s, "{"))
}
