package vim

import (
	"testing"

	"muse/editor/navigation"
)

func TestLinewiseOperatorRangeCountsSourceLines(t *testing.T) {
	source := "first\nsecond\nthird"
	cases := []struct {
		name     string
		caret    int
		count    int
		operator string
		want     navigation.Range
	}{
		{"single delete line", 2, 1, "d", navigation.Range{Start: 0, End: 5}},
		{"two delete lines include separator", 2, 2, "d", navigation.Range{Start: 0, End: 12}},
		{"count clamps to one", 2, 0, "y", navigation.Range{Start: 0, End: 5}},
		{"line starts at caret line", 8, 1, "y", navigation.Range{Start: 6, End: 12}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LinewiseOperatorRange(source, tc.caret, tc.count, tc.operator); got != tc.want {
				t.Fatalf("LinewiseOperatorRange() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestLinewiseChangeTrimsSelectedTrailingLF(t *testing.T) {
	source := "first\n\nthird"
	got := LinewiseOperatorRange(source, 2, 2, "c")
	want := navigation.Range{Start: 0, End: 5}
	if got != want {
		t.Fatalf("LinewiseOperatorRange(change) = %+v, want %+v", got, want)
	}
	if source[got.Start:got.End] != "first" {
		t.Fatalf("change range includes trailing LF: %q", source[got.Start:got.End])
	}
}

func TestLinewiseOperatorRangeHandlesFinalEmptyLine(t *testing.T) {
	source := "first\n"
	got := LinewiseOperatorRange(source, len(source), 1, "d")
	if got != (navigation.Range{Start: len(source), End: len(source)}) {
		t.Fatalf("final empty line range = %+v", got)
	}
}
