package mdoc

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestInlineMathLayout(t *testing.T) {
	d := Parse("质能方程 $E=mc^2$，分式 $\\frac{a}{b}$，以及 $\\sqrt{x+1}$。\n")
	tt := ui.NewTester(func(c *ui.Context) { View(c, d, Options{ContentWidth: 420}) }, 800, 400)
	tt.Frame()
	if !tt.HasText("质能方程") || !tt.HasText("以及") {
		t.Fatalf("text around math missing: %q", tt.Texts())
	}
}

func TestInlineMathPreservesExplicitHardBreaks(t *testing.T) {
	for _, tc := range []struct {
		src   string
		lines int
	}{
		{"before $x$\nafter", 1},
		{"before $x$  \nafter", 2},
		{"before $x$\\\nafter", 2},
	} {
		d := Parse(tc.src)
		tt := ui.NewTester(func(c *ui.Context) {
			s := rstyle{o: Options{ContentWidth: 800, SoftBreaksAsSpaces: true}.withDefaults()}
			l := layoutInlineMath(c, d.Blocks[0].Inlines, s, 16, 0, 1.7)
			if len(l.lines) != tc.lines {
				t.Fatalf("%q lines=%d want %d", tc.src, len(l.lines), tc.lines)
			}
		}, 1000, 400)
		tt.Frame()
	}
}
