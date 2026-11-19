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
