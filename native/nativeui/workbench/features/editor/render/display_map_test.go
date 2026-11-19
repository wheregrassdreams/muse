package render

import (
	"reflect"
	"testing"
)

type testPieceMap []MappedPiece

func (m testPieceMap) MappedPieceCount() int           { return len(m) }
func (m testPieceMap) MappedPieceAt(i int) MappedPiece { return m[i] }

func TestSourceAndDisplayMappingPreservesUTF8AndUTF16Boundaries(t *testing.T) {
	mapper := testPieceMap{
		{Kind: MappingText, SourceStart: 0, SourceEnd: 6, DisplayStart: 0, DisplayEnd: 4, Text: "a😀b"},
		{Kind: MappingAtom, SourceStart: 7, SourceEnd: 10, DisplayStart: 5, DisplayEnd: 7},
		{Kind: MappingDecoration, SourceStart: 12, SourceEnd: 14, DisplayStart: 7, DisplayEnd: 7},
	}
	if got, ok := SourceToDisplay(mapper, 5); !ok || got != 3 {
		t.Fatalf("SourceToDisplay(5) = %d,%v, want 3,true", got, ok)
	}
	if got, ok := SourceToDisplay(mapper, 7); !ok || got != 5 {
		t.Fatalf("atom start mapping = %d,%v", got, ok)
	}
	if got, ok := SourceToDisplay(mapper, 8); ok || got != 0 {
		t.Fatalf("atom interior mapping = %d,%v, want inaccessible", got, ok)
	}
	if got := DisplayToSource(mapper, 2, false); got != 5 {
		t.Fatalf("surrogate-interior display position = %d, want following rune boundary 5", got)
	}
	if got := DisplayToSource(mapper, 6, false); got != 7 {
		t.Fatalf("DisplayToSource(atom middle) = %d, want nearer start 7", got)
	}
	if got := DisplayToSource(mapper, 7, true); got != 10 {
		t.Fatalf("DisplayToSource(hidden syntax edge, up) = %d, want 10", got)
	}
	if got := DisplayToSource(mapper, 7, false); got != 12 {
		t.Fatalf("DisplayToSource(hidden syntax edge, down) = %d, want 12", got)
	}
	if got, want := CaretStops(mapper), []int{0, 1, 5, 6, 7, 10, 12}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CaretStops = %v, want %v", got, want)
	}
}

func TestDisplayMappingRetainsDecorationAndClampBehavior(t *testing.T) {
	mapper := testPieceMap{
		{Kind: MappingText, SourceStart: 0, SourceEnd: 1, DisplayStart: 0, DisplayEnd: 1, Text: "a"},
		{Kind: MappingDecoration, SourceStart: 4, SourceEnd: 8, DisplayStart: 1, DisplayEnd: 3, Text: "↗↗"},
		{Kind: MappingText, SourceStart: 9, SourceEnd: 10, DisplayStart: 3, DisplayEnd: 4, Text: "b"},
	}
	if got := DisplayToSource(mapper, 2, false); got != 4 {
		t.Fatalf("decoration interior = %d, want anchor 4", got)
	}
	if got, ok := SourceToDisplay(mapper, 6); ok || got != 0 {
		t.Fatalf("hidden source mapping = %d,%v, want inaccessible", got, ok)
	}
	if got := DisplayToSource(mapper, -1, false); got != 0 {
		t.Fatalf("negative display clamp = %d, want first source edge 0", got)
	}
	if got := DisplayToSource(mapper, 10, false); got != 10 {
		t.Fatalf("display-end clamp = %d, want final source edge 10", got)
	}
	if got, want := CaretStops(nil), []int{0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("empty display caret stops = %v, want %v", got, want)
	}
}
