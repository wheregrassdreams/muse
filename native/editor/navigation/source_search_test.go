package navigation

import (
	"strings"
	"testing"
)

func TestSearchRangesUsesSmartCaseGraphemesAndNonOverlappingMatches(t *testing.T) {
	source := "alpha ALPHA Alpha K k 👩‍💻x👩‍💻"
	got := SearchRanges(source, "alpha")
	if len(got) != 3 {
		t.Fatalf("case-insensitive SearchRanges returned %d ranges: %+v", len(got), got)
	}
	for _, r := range got {
		if source[r.Start:r.End] != "alpha" && source[r.Start:r.End] != "ALPHA" && source[r.Start:r.End] != "Alpha" {
			t.Fatalf("SearchRanges returned unexpected slice %q at %+v", source[r.Start:r.End], r)
		}
	}
	got = SearchRanges(source, "Alpha")
	if len(got) != 1 || source[got[0].Start:got[0].End] != "Alpha" {
		t.Fatalf("smart-case SearchRanges = %+v", got)
	}
	got = SearchRanges(source, "k")
	if len(got) != 2 || source[got[0].Start:got[0].End] != "K" || source[got[1].Start:got[1].End] != "k" {
		t.Fatalf("Unicode case-fold SearchRanges = %+v", got)
	}
	got = SearchRanges(source, "👩‍💻")
	if len(got) != 2 {
		t.Fatalf("grapheme SearchRanges = %+v", got)
	}
	for _, r := range got {
		if source[r.Start:r.End] != "👩‍💻" {
			t.Fatalf("SearchRanges split a grapheme: %+v", r)
		}
	}
	if got := SearchRanges("aaaaa", "aa"); len(got) != 2 || got[0] != (Range{0, 2}) || got[1] != (Range{2, 4}) {
		t.Fatalf("overlapping search highlights = %+v", got)
	}
}

func TestTextSearchBorrowsExistingGraphemeBoundaries(t *testing.T) {
	source := "start 👩‍💻 finish 👩‍💻"
	edges := GraphemeBoundaries(source)
	search := NewTextSearchWithBoundaries(source, edges)
	if len(search.edges) == 0 || &search.edges[0] != &edges[0] {
		t.Fatal("NewTextSearchWithBoundaries copied the grapheme boundary slice")
	}
	if matches := search.SearchRanges("👩‍💻"); len(matches) != 2 {
		t.Fatalf("borrowed-boundary SearchRanges() = %+v", matches)
	}
}

func TestSearchRangesCapsHighlightsAtFiveHundred(t *testing.T) {
	source := strings.Repeat("a", 1200)
	got := SearchRanges(source, "a")
	if len(got) != MaxSearchRanges || got[0] != (Range{0, 1}) || got[len(got)-1] != (Range{499, 500}) {
		t.Fatalf("SearchRanges cap/result = len %d, tail %+v", len(got), got[len(got)-1])
	}
}

func TestSearchWrapsAndCountsFromByteOffsets(t *testing.T) {
	source := "zero middle zero"
	first := 0
	second := strings.LastIndex(source, "zero")
	if got, ok := Search(source, "zero", first, Forward, 1); !ok || got != second {
		t.Fatalf("forward Search() = %d,%v; want %d,true", got, ok, second)
	}
	if got, ok := Search(source, "zero", second, Forward, 1); !ok || got != first {
		t.Fatalf("forward wrap Search() = %d,%v; want %d,true", got, ok, first)
	}
	if got, ok := Search(source, "zero", first, Backward, 1); !ok || got != second {
		t.Fatalf("backward wrap Search() = %d,%v; want %d,true", got, ok, second)
	}
	if got, ok := Search(source, "zero", first, Forward, 2); !ok || got != first {
		t.Fatalf("counted Search() = %d,%v; want %d,true", got, ok, first)
	}
	if got, ok := Search(source, "missing", first, Forward, 1); ok || got != first {
		t.Fatalf("missing Search() = %d,%v; want unchanged offset,false", got, ok)
	}
}

func TestFindSearchesOneLineByGraphemeAndSupportsTill(t *testing.T) {
	source := "a👩‍💻bxb\nb"
	emoji := strings.Index(source, "👩‍💻")
	if got, ok := Find(source, "b", 0, Forward, false, 2); !ok || got != strings.Index(source, "bxb")+2 {
		t.Fatalf("counted Find() = %d,%v", got, ok)
	}
	if got, ok := Find(source, "b", 0, Forward, true, 1); !ok || got != emoji {
		t.Fatalf("forward till Find() = %d,%v; want %d,true", got, ok, emoji)
	}
	if got, ok := Find(source, "a", len("a👩‍💻bxb"), Backward, true, 1); !ok || got != len("a") {
		t.Fatalf("backward till Find() = %d,%v", got, ok)
	}
	if got, ok := Find(source, "b", 0, Backward, false, 1); ok || got != 0 {
		t.Fatalf("Find() crossed line boundary: %d,%v", got, ok)
	}
	if got, ok := Find("👩‍💻 👩‍💻", "👩‍💻", 0, Forward, false, 1); !ok || got != len("👩‍💻 ") {
		t.Fatalf("grapheme Find() = %d,%v", got, ok)
	}
}
