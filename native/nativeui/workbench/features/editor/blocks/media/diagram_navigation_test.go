package media

import (
	"math"
	"testing"
)

func TestDiagramScrollOffsetsStayWithinContentBounds(t *testing.T) {
	if got := DiagramPanOffset(50, 10, -10, 100); got != 70 {
		t.Fatalf("pan offset = %v, want 70", got)
	}
	if got := DiagramScrollOffset(95, 20, 100); got != 100 {
		t.Fatalf("scroll offset = %v, want 100", got)
	}
	if got := DiagramScrollOffset(5, -20, 100); got != 0 {
		t.Fatalf("reverse scroll offset = %v, want 0", got)
	}
}

func TestDiagramScrollbarPressAndDragMapTrackToContent(t *testing.T) {
	if got := DiagramBarPressOffset(30, 30, 10, 100, 20, 30, 200, true); got != 30 {
		t.Fatalf("press on thumb offset = %v, want unchanged 30", got)
	}
	if got := DiagramBarPressOffset(30, 90, 10, 100, 20, 30, 200, false); math.Abs(float64(got-185.71428)) > .001 {
		t.Fatalf("track press offset = %v, want centered position 185.71428", got)
	}
	if got := DiagramBarDragOffset(0, 10, 50, 200, 100, 30); math.Abs(float64(got-114.28571)) > .001 {
		t.Fatalf("drag offset = %v, want 114.28571", got)
	}
	if got := DiagramBarDragOffset(10, 0, 10, 200, 20, 20); got != 0 {
		t.Fatalf("zero-travel track offset = %v, want 0", got)
	}
}
