package markdown

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestViewRendersSequence(t *testing.T) {
	d := Parse("```mermaid\nsequenceDiagram\nAlice->>Bob: Hi\n```\n")
	tt := ui.NewTester(func(c *ui.Context) { View(c, d, Options{}) }, 800, 400)
	tt.Frame()
}
