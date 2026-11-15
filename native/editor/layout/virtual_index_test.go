package layout

import "testing"

func TestVirtualIndexAppliesMeasuredHeightDeltas(t *testing.T) {
	var index VirtualIndex
	index.Capture([]float32{0, 10, 20, 30}, 40)
	index.AddHeight(1, 5)
	index.AddHeight(2, -2)
	if got := index.Y(3); got != 33 {
		t.Fatalf("Y(3) = %v, want 33", got)
	}
	if got := index.DocumentHeight(); got != 43 {
		t.Fatalf("DocumentHeight() = %v, want 43", got)
	}
}

func TestVirtualIndexVisibleRangeIncludesIntersectingBlocks(t *testing.T) {
	var index VirtualIndex
	index.Capture([]float32{0, 10, 20, 30}, 40)
	heights := []float32{10, 10, 10, 10}
	start, end := index.VisibleRange(11, 20, func(i int) float32 { return heights[i] })
	if start != 1 || end != 3 {
		t.Fatalf("VisibleRange() = [%d,%d), want [1,3)", start, end)
	}
}

func TestVirtualIndexResetKeepsReusableCapacity(t *testing.T) {
	var index VirtualIndex
	index.Capture([]float32{0, 10, 20}, 30)
	baseCapacity := cap(index.baseY)
	index.Reset()
	if index.Ready(0) || index.Len() != 0 || index.DocumentHeight() != 0 {
		t.Fatal("Reset() left a ready or non-empty index")
	}
	index.Capture([]float32{0, 4}, 8)
	if cap(index.baseY) != baseCapacity {
		t.Fatal("Capture() did not reuse capacity after reset")
	}
}

func TestVirtualIndexIgnoresOutOfRangeCorrectionsAndClampsPrefixes(t *testing.T) {
	var index VirtualIndex
	index.Capture([]float32{0, 10}, 20)
	index.AddHeight(-1, 100)
	index.AddHeight(2, 100)
	if got := index.Prefix(-1); got != 0 {
		t.Fatalf("Prefix(-1) = %v, want 0", got)
	}
	if got := index.Prefix(100); got != 0 {
		t.Fatalf("Prefix(100) = %v, want 0 with no valid corrections", got)
	}
	index.CaptureCount(-3, 9)
	if index.Len() != 0 || index.DocumentHeight() != 9 {
		t.Fatalf("negative CaptureCount left len=%d height=%v", index.Len(), index.DocumentHeight())
	}
}
