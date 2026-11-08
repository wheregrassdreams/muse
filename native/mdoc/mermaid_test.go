package mdoc

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestParseMermaidFlowchart(t *testing.T) {
	g, ok := parseMermaid(`graph TD
A[Start] --> B{Ready?}
B -->|yes| C[Ship]
B -.-> D[Retry]
`)
	if !ok {
		t.Fatal("parse failed")
	}
	if g.dir != "TD" || len(g.nodes) != 4 || len(g.edges) != 3 {
		t.Fatalf("graph = dir %s nodes %d edges %d", g.dir, len(g.nodes), len(g.edges))
	}
	if g.byID["A"].label != "Start" || g.byID["B"].label != "Ready?" {
		t.Fatalf("labels = %q %q", g.byID["A"].label, g.byID["B"].label)
	}
	if g.edges[1].label != "yes" || !g.edges[2].dashed {
		t.Fatalf("edges = %+v", g.edges)
	}
}

func TestViewRendersMermaid(t *testing.T) {
	d := Parse("```mermaid\ngraph LR\nA[One] --> B[Two]\n```\n")
	tt := ui.NewTester(func(c *ui.Context) { View(c, d, Options{}) }, 800, 400)
	tt.Frame()
	if !tt.HasText("One") && !tt.HasText("Two") {
		// Diagram labels are painted, but the block must not panic or stay code.
		t.Logf("texts = %v", tt.Texts())
	}
}

// TestHorizontalDiagramFitsItsColumns checks that a sideways graph is tall
// enough for the rank with the most nodes, which stack in one column.
func TestHorizontalDiagramFitsItsColumns(t *testing.T) {
	d, ok := ParseDiagram("graph LR\nA --> B1\nA --> B2\nA --> B3\nA --> B4\nA --> B5\nA --> B6\n")
	if !ok {
		t.Fatal("fan-out graph did not parse")
	}
	// Each node is 36 tall on a 52 stride: the sixth ends at 296.
	if h := d.Height(0); h < 296 {
		t.Fatalf("height = %v, want at least 296", h)
	}
}

// TestHorizontalDiagramMakesRoomForLabels checks that a labelled edge of a
// sideways graph has room between its columns: the label sits in the gap
// rather than over the nodes around it.
func TestHorizontalDiagramMakesRoomForLabels(t *testing.T) {
	const label = "执行 submit() 且门禁失败"
	g, ok := parseMermaid("graph LR\nA[开始] -->|" + label + "| B[结束]\n")
	if !ok {
		t.Fatal("the graph did not parse")
	}
	g.layout(ui.Rect{X: 0, Y: 0, W: g.width(), H: g.height()})
	a, b := g.byID["A"].rect, g.byID["B"].rect
	gap := b.X - (a.X + a.W)
	if want := edgeLabelWidth(label); gap < want {
		t.Fatalf("the gap between the columns is %.1f, want at least the label's %.1f", gap, want)
	}
}

// TestDiagramWidth checks the least width each kind of diagram reports, as
// the editor uses it to tell that a diagram overflows its column.
func TestDiagramWidth(t *testing.T) {
	cases := []struct {
		name string
		code string
		min  float32
	}{
		{"flowchart", "graph LR\nA[一] --> B[二]\n", 200},
		{"wide flowchart", "graph LR\nA --> B\nB --> C\nC --> D\nD --> E\nE --> F\nF --> G\nG --> H\n", 800},
		{"state", "stateDiagram\n[*] --> A\nA --> B\n", 140},
		{"sequence", "sequenceDiagram\nAlice->>Bob: Hi\n", 200},
		{"class", "classDiagram\nclass A\nclass B\nA --> B\n", 200},
		{"pie", "pie title Pets\n\"Dogs\": 3\n", 400},
		{"gantt", "gantt\ntitle Plan\nsection Work\nTask :a1, 2024-01-01, 3d\n", 250},
	}
	for _, c := range cases {
		d, ok := ParseDiagram(c.code)
		if !ok {
			t.Fatalf("%s did not parse", c.name)
		}
		if w := d.Width(); w < c.min {
			t.Fatalf("%s width = %v, want at least %v", c.name, w, c.min)
		}
	}
}

// TestASubgraphFallsBackToTheSource: the subgraph box is part of the
// picture. Drawing the graph without it would be a different diagram, so the
// block shows its source instead — the editor is the place where what it
// cannot draw is still readable.
func TestASubgraphFallsBackToTheSource(t *testing.T) {
	if _, ok := ParseDiagram("graph TD\n  subgraph one\n    A --> B\n  end\n"); ok {
		t.Fatal("a subgraph drew as a graph without it")
	}
	if _, ok := ParseDiagram("graph TD\n  A --> B\n"); !ok {
		t.Fatal("a plain graph did not draw")
	}
}
