package editor

import (
	"testing"
)

func TestVirtualScrollHeightIndexKeepsViewportShaped(t *testing.T) {
	e := New(stressContent("prose", 240), Options{FontSize: 16})
	tt := stressTester(e)
	tt.Frame()
	tt.Frame()
	if !e.virtualIndex.Ready(len(e.visibleLeaves)) || e.virtualIndex.Len() != len(e.visibleLeaves) {
		t.Fatal("virtual scroll height index was not initialized")
	}

	// Unvisited leaves keep estimated heights, so the document extent is
	// intentionally approximate until those leaves enter the layout band.
	// Exercise large forward/backward jumps and verify that corrections never
	// leave an estimated paragraph inside the actual viewport.
	for _, fraction := range []float32{0.72, 0.24, 0.91, 0.48} {
		maxScroll := max(e.docH-e.viewH, 0)
		e.scroll.Y = fraction * maxScroll
		tt.Frame()

		if e.visibleLeafNeedsShape() {
			t.Fatalf("scroll %.2f left an unshaped leaf in the viewport", fraction)
		}
		previousY := float32(-1)
		for i, leaf := range e.visibleLeaves {
			y := e.virtualY(i)
			if y < previousY {
				t.Fatalf("height index is not ordered at leaf %d: y=%f after %f", i, y, previousY)
			}
			previousY = y
			if leaf.kind == kPara && y+leaf.h > e.scroll.Y && y < e.scroll.Y+e.viewH && leaf.view == nil {
				t.Fatalf("visible paragraph %d at y=%.1f h=%.1f is still estimated", i, y, leaf.h)
			}
		}
		indexedHeight := e.virtualIndex.BaseDocumentHeight() + e.virtualHeightPrefix(len(e.visibleLeaves))
		if delta := indexedHeight - e.docH; delta < -0.05 || delta > 0.05 {
			t.Fatalf("indexed document height=%f, reported height=%f", indexedHeight, e.docH)
		}
	}
}
