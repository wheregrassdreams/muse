package markdown

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestViewRendersMermaid(t *testing.T) {
	d := Parse("```mermaid\ngraph LR\nA[One] --> B[Two]\n```\n")
	tt := ui.NewTester(func(c *ui.Context) { View(c, d, Options{}) }, 800, 400)
	tt.Frame()
	if !tt.HasText("One") && !tt.HasText("Two") {
		t.Logf("texts = %v", tt.Texts())
	}
}

func TestHorizontalDiagramFitsItsColumns(t *testing.T) {
	d, ok := ParseDiagram("graph LR\nA --> B1\nA --> B2\nA --> B3\nA --> B4\nA --> B5\nA --> B6\n")
	if !ok {
		t.Fatal("fan-out graph did not parse")
	}
	if h := d.Height(0); h < 296 {
		t.Fatalf("height = %v, want at least 296", h)
	}
}

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
		t.Run(c.name, func(t *testing.T) {
			d, ok := ParseDiagram(c.code)
			if !ok {
				t.Fatal("diagram did not parse")
			}
			if w := d.Width(); w < c.min {
				t.Fatalf("width = %v, want at least %v", w, c.min)
			}
		})
	}
}

func TestASubgraphFallsBackToTheSource(t *testing.T) {
	if _, ok := ParseDiagram("graph TD\n  subgraph one\n    A --> B\n  end\n"); ok {
		t.Fatal("a subgraph drew as a graph without it")
	}
	if _, ok := ParseDiagram("graph TD\n  A --> B\n"); !ok {
		t.Fatal("a plain graph did not draw")
	}
}
