package markdown

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestParseSequenceDiagram(t *testing.T) {
	g, ok := parseSequence(`sequenceDiagram
participant A as Alice
participant B as Bob
A->>B: Hello
B-->>A: Hi
Note over A,B: pair
`)
	if !ok || len(g.parts) != 2 || len(g.events) != 3 {
		t.Fatalf("graph = %+v", g)
	}
	if g.byID["A"].label != "Alice" || g.events[0].text != "Hello" || !g.events[1].dashed {
		t.Fatalf("events = %+v", g.events)
	}
}

func TestViewRendersSequence(t *testing.T) {
	d := Parse("```mermaid\nsequenceDiagram\nAlice->>Bob: Hi\n```\n")
	tt := ui.NewTester(func(c *ui.Context) { View(c, d, Options{}) }, 800, 400)
	tt.Frame()
}
