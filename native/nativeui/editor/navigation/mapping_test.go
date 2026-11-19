package navigation

import (
	"reflect"
	"testing"
)

func TestMapConvertsUTF16VisibleOffsetsToUTF8SourceOffsets(t *testing.T) {
	m := Map{Pieces: []Piece{{
		Kind: Text, Source: Offset{Start: 4, End: 12}, Visible: Offset{Start: 0, End: 4},
		SourceText: "A😀中", VisibleText: "A😀中",
	}}}
	for _, tc := range []struct{ visible, source int }{{0, 4}, {1, 5}, {3, 9}, {4, 12}} {
		got, ok := m.SourceAt(tc.visible, TowardStart)
		if !ok || got != tc.source {
			t.Fatalf("SourceAt(%d) = %d, %v; want %d, true", tc.visible, got, ok, tc.source)
		}
		got, ok = m.VisibleAt(tc.source, TowardStart)
		if !ok || got != tc.visible {
			t.Fatalf("VisibleAt(%d) = %d, %v; want %d, true", tc.source, got, ok, tc.visible)
		}
	}
	if got, ok := m.SourceAt(2, TowardStart); !ok || got != 9 {
		t.Fatalf("surrogate-interior mapping = %d, %v; want following rune boundary 9", got, ok)
	}
}

type testPieceProvider []Piece

func (p testPieceProvider) PieceCount() int     { return len(p) }
func (p testPieceProvider) PieceAt(i int) Piece { return p[i] }

func TestProviderBackedMapPreservesDecorationAndCaretSemantics(t *testing.T) {
	m := Map{Provider: testPieceProvider{
		{Kind: Text, Source: Offset{Start: 0, End: 1}, Visible: Offset{Start: 0, End: 1}, SourceText: "a", VisibleText: "a"},
		{Kind: Decoration, Source: Offset{Start: 4, End: 8}, Visible: Offset{Start: 1, End: 3}},
		{Kind: Atom, Source: Offset{Start: 9, End: 12}, Visible: Offset{Start: 3, End: 4}},
	}}
	if got, ok := m.SourceAt(2, TowardEnd); !ok || got != 4 {
		t.Fatalf("decoration interior = %d, %v; want anchor 4", got, ok)
	}
	if got, want := m.CaretStops(), []int{0, 1, 4, 9, 12}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CaretStops() = %v, want %v", got, want)
	}
}

func TestZeroSourceDecorationMapsVisibleTextToAnchorButNotBack(t *testing.T) {
	m := Map{Pieces: []Piece{{
		Kind: Decoration, Source: Offset{Start: 5, End: 5}, Visible: Offset{Start: 2, End: 4},
	}}}
	if got, ok := m.SourceAt(3, TowardStart); !ok || got != 5 {
		t.Fatalf("visible decoration interior = %d, %v; want source anchor 5", got, ok)
	}
	if got, ok := m.VisibleAt(5, TowardStart); ok {
		t.Fatalf("zero-range decoration anchor mapped back to display %d; want no source caret stop", got)
	}
}

func TestMapResolvesAtomsAndHiddenSyntaxByBias(t *testing.T) {
	m := Map{Pieces: []Piece{
		{Kind: Text, Source: Offset{Start: 0, End: 1}, Visible: Offset{Start: 0, End: 1}, SourceText: "a", VisibleText: "a"},
		{Kind: Atom, Source: Offset{Start: 3, End: 9}, Visible: Offset{Start: 1, End: 2}},
		{Kind: Text, Source: Offset{Start: 12, End: 13}, Visible: Offset{Start: 2, End: 3}, SourceText: "b", VisibleText: "b"},
	}}
	if got, ok := m.SourceAt(1, TowardStart); !ok || got != 1 {
		t.Fatalf("start-biased hidden-syntax mapping = %d, %v", got, ok)
	}
	if got, ok := m.SourceAt(1, TowardEnd); !ok || got != 3 {
		t.Fatalf("end-biased hidden-syntax mapping = %d, %v", got, ok)
	}
	if got, ok := m.SourceAt(2, TowardEnd); !ok || got != 12 {
		t.Fatalf("atom edge mapping = %d, %v; want next source run", got, ok)
	}
	if got, ok := m.SourceAt(1, TowardStart); !ok || got != 1 {
		t.Fatalf("atom start mapping = %d, %v", got, ok)
	}
	if got, ok := (Map{Pieces: []Piece{{Kind: Atom, Source: Offset{Start: 3, End: 9}, Visible: Offset{Start: 10, End: 14}}}}).SourceAt(12, TowardStart); !ok || got != 3 {
		t.Fatalf("atom interior nearest endpoint = %d, %v; want 3", got, ok)
	}
}

func TestMapRejectsInconsistentTextPieceOffsets(t *testing.T) {
	m := Map{Pieces: []Piece{{
		Kind: Text, Source: Offset{Start: 0, End: 2}, Visible: Offset{Start: 0, End: 1},
		SourceText: "abc", VisibleText: "abc",
	}}}
	if _, ok := m.SourceAt(0, TowardStart); ok {
		t.Fatal("accepted inconsistent text-piece source offsets")
	}
}
