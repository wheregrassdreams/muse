package diagrams

import "testing"

func TestParseFlowchartAndLayout(t *testing.T) {
	chart, ok := ParseFlowchart(`graph TD
A[Start] --> B{Ready?}
B -->|yes| C[Ship]
B -.-> D[Retry]
`, FlowchartOptions{StartLabel: "Begin"})
	if !ok || chart.Direction != "TD" || len(chart.Nodes) != 4 || len(chart.Edges) != 3 {
		t.Fatalf("flowchart = %+v", chart)
	}
	if chart.Nodes[0].Label != "Start" || chart.Nodes[1].Label != "Ready?" {
		t.Fatalf("labels = %+v", chart.Nodes)
	}
	if chart.Edges[1].Label != "yes" || !chart.Edges[2].Dashed {
		t.Fatalf("edges = %+v", chart.Edges)
	}
	if chart.Nodes[0].Level != 0 || chart.Nodes[1].Level != 1 || chart.Nodes[2].Level != 2 {
		t.Fatalf("rank levels = %+v", chart.Nodes)
	}
	layout := chart.Layout(chart.Width(), chart.Height())
	if len(layout) != len(chart.Nodes) || layout[0].Rect.W <= 0 || layout[2].Rect.Y <= layout[0].Rect.Y {
		t.Fatalf("layout = %+v", layout)
	}
}

func TestFlowchartHorizontalRankGapFitsCJKLabel(t *testing.T) {
	const label = "执行 submit() 且门禁失败"
	chart, ok := ParseFlowchart("graph LR\nA[开始] -->|"+label+"| B[结束]\n", FlowchartOptions{})
	if !ok {
		t.Fatal("flowchart did not parse")
	}
	layout := chart.Layout(chart.Width(), chart.Height())
	gap := layout[1].Rect.X - (layout[0].Rect.X + layout[0].Rect.W)
	if want := edgeLabelWidth(label) + 16; gap < want {
		t.Fatalf("rank gap = %.1f, want at least %.1f", gap, want)
	}
}

func TestStateDiagramFlowchartUsesSharedLayout(t *testing.T) {
	state, ok := ParseState("stateDiagram\n[*] --> Idle\nIdle --> Done\n", StateOptions{StartLabel: "Start"})
	if !ok {
		t.Fatal("state diagram did not parse")
	}
	chart := state.Flowchart()
	if len(chart.Layout(chart.Width(), chart.Height())) != 3 {
		t.Fatal("state model did not produce shared flowchart geometry")
	}
}
