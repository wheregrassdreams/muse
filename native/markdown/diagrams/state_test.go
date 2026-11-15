package diagrams

import "testing"

func TestParseStateDiagram(t *testing.T) {
	g, ok := ParseState(`stateDiagram-v2
direction LR
state "Idle now" as Idle
[*] --> Idle
Idle --> Running: start
Running --> [*]: stop
`, StateOptions{StartLabel: "Start", EndLabel: "End"})
	if !ok || g.Direction != "LR" || len(g.Nodes) != 4 || len(g.Edges) != 3 {
		t.Fatalf("state diagram = %+v", g)
	}
	labels := map[string]string{}
	for _, node := range g.Nodes {
		labels[node.ID] = node.Label
	}
	if labels["[*]start"] != "Start" || labels["Idle"] != "Idle now" || labels["[*]end"] != "End" {
		t.Fatalf("nodes = %+v", g.Nodes)
	}
	if g.Edges[1].Label != "start" {
		t.Fatalf("edge = %+v", g.Edges[1])
	}
}
