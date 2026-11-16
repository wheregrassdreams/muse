package navigation

import "testing"

func TestBigWordStartPreservesHalfOpenDirectionalBoundaries(t *testing.T) {
	ranges := []Range{{Start: 1, End: 4}, {Start: 6, End: 8}, {Start: 10, End: 14}}
	for _, tc := range []struct {
		name      string
		offset    int
		direction Direction
		want      Range
		found     bool
	}{
		{"forward before first", 0, Forward, ranges[0], true},
		{"forward inside first is strict", 2, Forward, ranges[1], true},
		{"forward at first end", 4, Forward, ranges[1], true},
		{"forward at second start is strict", 6, Forward, ranges[2], true},
		{"forward at final end", 14, Forward, Range{}, false},
		{"backward before first", 1, Backward, Range{}, false},
		{"backward inside first", 2, Backward, ranges[0], true},
		{"backward at first end", 4, Backward, ranges[0], true},
		{"backward at second start", 6, Backward, ranges[0], true},
		{"backward after final", 15, Backward, ranges[2], true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := BigWordStart(ranges, tc.offset, tc.direction)
			if ok != tc.found || got != tc.want {
				t.Fatalf("BigWordStart(%d, %d) = %+v,%v; want %+v,%v", tc.offset, tc.direction, got, ok, tc.want, tc.found)
			}
		})
	}
}

func TestBigWordEndPreservesCurrentAndNextBoundaries(t *testing.T) {
	ranges := []Range{{Start: 1, End: 4}, {Start: 6, End: 8}, {Start: 10, End: 14}}
	for _, tc := range []struct {
		name   string
		offset int
		next   bool
		want   Range
		found  bool
	}{
		{"before first", 0, false, ranges[0], true},
		{"inside first", 2, false, ranges[0], true},
		{"at first end", 4, false, ranges[1], true},
		{"gap before second", 5, false, ranges[1], true},
		{"at second start", 6, false, ranges[1], true},
		{"next skips containing range", 2, true, ranges[1], true},
		{"next at first end", 4, true, ranges[1], true},
		{"next at second start", 6, true, ranges[2], true},
		{"after final", 14, false, Range{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := BigWordEnd(ranges, tc.offset, tc.next)
			if ok != tc.found || got != tc.want {
				t.Fatalf("BigWordEnd(%d, %v) = %+v,%v; want %+v,%v", tc.offset, tc.next, got, ok, tc.want, tc.found)
			}
		})
	}
}
