package shell

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLayoutAppliesShellCursorPolicy(t *testing.T) {
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame { return Frame{HideMouseCursor: true} },
	})
	tester := ui.NewTester(layout.Render, 800, 600)
	tester.Move(400, 300)
	tester.Frame()
	if got := tester.Cursor(); got != ui.CursorNone {
		t.Fatalf("hidden cursor = %v, want none", got)
	}
}

func TestFlatWindowBackdropDoesNotDimEditorContent(t *testing.T) {
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame { return Frame{FlatCanvas: true, WindowBlur: true, WindowOpacity: 0.7} },
		RenderCanvas: func(c *ui.Context, _ bool) {
			ui.Text(c, "X").Absolute().Left(100).Top(100).FontSize(42).TextColor(ui.Hex("#ffffff"))
		},
	})
	tester := ui.NewTester(layout.Render, 420, 300)
	tester.Frame()
	image := tester.Image()
	bright := 0
	for y := 95; y < 155; y++ {
		for x := 95; x < 140; x++ {
			r, g, b, _ := image.At(x, y).RGBA()
			if r > 56000 && g > 56000 && b > 56000 {
				bright++
			}
		}
	}
	if bright == 0 {
		t.Fatal("translucent window background dimmed the editor glyph")
	}
}
