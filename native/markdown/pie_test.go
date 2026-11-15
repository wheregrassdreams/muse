package markdown

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestParsePie(t *testing.T) {
	g, ok := parsePie(`pie title 流量
"Dogs" : 386
"Cats" : 85
"Rats" : 15
`)
	if !ok || g.title != "流量" || len(g.slices) != 3 || g.slices[0].label != "Dogs" || g.slices[0].value != 386 {
		t.Fatalf("pie = %+v", g)
	}
}

func TestViewRendersPie(t *testing.T) {
	d := Parse("```mermaid\npie title Pets\n\"Dogs\": 3\n\"Cats\": 1\n```\n")
	tt := ui.NewTester(func(c *ui.Context) { View(c, d, Options{}) }, 800, 400)
	tt.Frame()
}
