package render

import "testing"

func TestVisibleRangeUsesHalfOpenIndexIntervalAndIncludesBoundary(t *testing.T) {
	blocks := []VerticalBounds{{Y: 0, H: 10}, {Y: 10, H: 10}, {Y: 20, H: 10}, {Y: 30, H: 10}}
	boundsAt := func(i int) VerticalBounds { return blocks[i] }
	cases := []struct {
		name        string
		top, bottom float32
		start, end  int
	}{
		{name: "middle block", top: 11, bottom: 19, start: 1, end: 2},
		{name: "touching bounds", top: 10, bottom: 20, start: 0, end: 3},
		{name: "after document", top: 50, bottom: 60, start: 4, end: 4},
		{name: "before document", top: -20, bottom: -1, start: 0, end: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end := VisibleRange(len(blocks), boundsAt, tc.top, tc.bottom)
			if start != tc.start || end != tc.end {
				t.Fatalf("VisibleRange = [%d,%d), want [%d,%d)", start, end, tc.start, tc.end)
			}
		})
	}
}

func TestVisibleRangeHandlesEmptyAndInvalidInput(t *testing.T) {
	if start, end := VisibleRange(0, func(int) VerticalBounds { return VerticalBounds{} }, 0, 1); start != 0 || end != 0 {
		t.Fatalf("empty range = [%d,%d)", start, end)
	}
	if start, end := VisibleRange(2, nil, 0, 1); start != 0 || end != 0 {
		t.Fatalf("nil callback range = [%d,%d)", start, end)
	}
	if start, end := VisibleRange(2, func(int) VerticalBounds { return VerticalBounds{} }, 2, 1); start != 0 || end != 0 {
		t.Fatalf("reversed viewport range = [%d,%d)", start, end)
	}
}
