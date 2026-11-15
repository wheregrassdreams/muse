package diagrams

import "strings"

// StateNode is one state identifier and its visible label.
type StateNode struct {
	ID, Label string
}

// StateEdge links two states and may carry a transition label.
type StateEdge struct {
	From, To string
	Label    string
}

// StateDiagram is the backend-neutral state transition model.
type StateDiagram struct {
	Direction string
	Nodes     []StateNode
	Edges     []StateEdge
}

// StateOptions injects localized labels for Mermaid's initial/final markers.
type StateOptions struct{ StartLabel, EndLabel string }

// Flowchart converts a state diagram into the shared rank and edge layout
// model used by the native Mermaid painter.
func (g *StateDiagram) Flowchart() *Flowchart {
	model := &Flowchart{Direction: g.Direction, Nodes: make([]FlowNode, len(g.Nodes))}
	indices := make(map[string]int, len(g.Nodes))
	for i, node := range g.Nodes {
		model.Nodes[i] = FlowNode{ID: node.ID, Label: node.Label}
		indices[node.ID] = i
	}
	for _, edge := range g.Edges {
		from, fromOK := indices[edge.From]
		to, toOK := indices[edge.To]
		if fromOK && toOK {
			model.Edges = append(model.Edges, FlowEdge{From: from, To: to, Label: edge.Label})
		}
	}
	model.assignLevels()
	return model
}

// ParseState parses simple transitions from Mermaid stateDiagram source.
// Composite state bodies, notes, class definitions and styles are ignored;
// plain transitions within those bodies remain visible.
func ParseState(source string, opts StateOptions) (*StateDiagram, bool) {
	lines := strings.Split(source, "\n")
	if len(lines) == 0 {
		return nil, false
	}
	head := strings.ToLower(strings.TrimSpace(lines[0]))
	if head != "statediagram" && head != "statediagram-v2" {
		return nil, false
	}
	diagram := &StateDiagram{Direction: "TD"}
	byID := make(map[string]int)
	getNode := func(id, label string) {
		if id == "" {
			return
		}
		if i, ok := byID[id]; ok {
			if label != "" {
				diagram.Nodes[i].Label = label
			}
			return
		}
		if label == "" {
			label = id
		}
		byID[id] = len(diagram.Nodes)
		diagram.Nodes = append(diagram.Nodes, StateNode{ID: id, Label: label})
	}
	for _, raw := range lines[1:] {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "direction ") {
			dir := strings.ToUpper(strings.TrimSpace(line[len("direction "):]))
			if dir == "LR" || dir == "RL" || dir == "TD" || dir == "TB" || dir == "BT" {
				diagram.Direction = dir
			}
			continue
		}
		if strings.HasPrefix(low, "state ") {
			rest := strings.TrimSpace(line[len("state "):])
			if strings.HasPrefix(rest, `"`) {
				if end := strings.Index(rest[1:], `"`); end >= 0 {
					label := rest[1 : end+1]
					after := strings.TrimSpace(rest[end+2:])
					if strings.HasPrefix(strings.ToLower(after), "as ") {
						getNode(strings.TrimSpace(after[3:]), label)
					}
				}
				continue
			}
			id := rest
			if i := strings.IndexAny(id, " {"); i >= 0 {
				id = id[:i]
			}
			getNode(strings.TrimSpace(id), "")
			continue
		}
		skip := false
		for _, prefix := range []string{"note ", "classdef", "class ", "style "} {
			if strings.HasPrefix(low, prefix) {
				skip = true
				break
			}
		}
		if skip || line == "}" {
			continue
		}
		at := strings.Index(line, "-->")
		if at < 0 {
			continue
		}
		left := strings.TrimSpace(line[:at])
		right := strings.TrimSpace(line[at+3:])
		label := ""
		if colon := strings.Index(right, ":"); colon >= 0 {
			label = strings.TrimSpace(right[colon+1:])
			right = strings.TrimSpace(right[:colon])
		}
		fromID, toID := left, right
		if fromID == "[*]" {
			fromID = "[*]start"
			if opts.StartLabel != "" {
				getNode(fromID, opts.StartLabel)
			}
		}
		if toID == "[*]" {
			toID = "[*]end"
			if opts.EndLabel != "" {
				getNode(toID, opts.EndLabel)
			}
		}
		getNode(fromID, "")
		getNode(toID, "")
		diagram.Edges = append(diagram.Edges, StateEdge{From: fromID, To: toID, Label: label})
	}
	if len(diagram.Nodes) == 0 || len(diagram.Edges) == 0 {
		return nil, false
	}
	return diagram, true
}
