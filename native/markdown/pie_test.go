package markdown

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/markdown/diagrams"
)

func TestParsePie(t *testing.T) {
	g, ok := diagrams.ParsePie(`pie title 流量
"Dogs" : 386
"Cats" : 85
"Rats" : 15
`)
	if !ok || g.Title != "流量" || len(g.Slices) != 3 || g.Slices[0].Label != "Dogs" || g.Slices[0].Value != 386 {
		t.Fatalf("pie = %+v", g)
	}
}

func TestViewRendersPie(t *testing.T) {
	d := Parse("```mermaid\npie title Pets\n\"Dogs\": 3\n\"Cats\": 1\n```\n")
	tt := ui.NewTester(func(c *ui.Context) { View(c, d, Options{}) }, 800, 400)
	tt.Frame()
}
