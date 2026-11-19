package editor

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestFirstPaintUsesResolvedWidthForColumnPadding(t *testing.T) {
	e := New("# Heading\n", Options{MaxWidth: 820, PadX: 40})
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { e.View(c).Grow(1) })
		// Reproduce the initial frame where the scroll handle reports no size
		// during View, but the draw callback receives the resolved bounds.
		e.elemW, e.colX, e.colW = 0, 0, 720
		e.leavesLaid = true
	}, 1000, 700)

	tester.Frame()

	width := e.elemW
	wantColumnWidth := max(min(float32(820), width-2*40), float32(120))
	wantColumnX := max((width-wantColumnWidth)/2, float32(0))
	if width <= 0 || e.colW != wantColumnWidth || e.colX != wantColumnX || e.colX <= 0 {
		t.Fatalf("first paint geometry = width %.1f, column x %.1f/width %.1f; want x %.1f/width %.1f with visible side padding", width, e.colX, e.colW, wantColumnX, wantColumnWidth)
	}
}
