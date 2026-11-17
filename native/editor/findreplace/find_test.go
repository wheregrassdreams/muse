package findreplace

import (
	"strings"
	"testing"
)

func TestFindUsesOriginalUTF8ByteRangesAndLineSnippets(t *testing.T) {
	source := "第一行\n这里 Muse 和 muse\n最后一行"
	matches := Find(source, "MUSE")
	if len(matches) != 2 {
		t.Fatalf("matches = %#v, want 2", matches)
	}
	for _, match := range matches {
		if source[match.Start:match.End] != "Muse" && source[match.Start:match.End] != "muse" {
			t.Fatalf("match range = %d:%d (%q)", match.Start, match.End, source[match.Start:match.End])
		}
		if match.Snippet != "这里 Muse 和 muse" {
			t.Fatalf("snippet = %q", match.Snippet)
		}
	}
}

func TestFindCapsResultsAndIgnoresBlankQuery(t *testing.T) {
	if got := Find("abc", " \t "); got != nil {
		t.Fatalf("blank query matches = %#v", got)
	}
	source := make([]byte, 0, MaxMatches+1)
	for i := 0; i < MaxMatches+1; i++ {
		source = append(source, 'x')
	}
	matches := Find(string(source), "x")
	if len(matches) != MaxMatches {
		t.Fatalf("match count = %d, want %d", len(matches), MaxMatches)
	}
}

func TestFindPreservesSourceOffsetsWhenLowercaseChangesRuneWidth(t *testing.T) {
	source := "before İSTANBUL, after"
	matches := Find(source, "istanbul")
	if len(matches) != 1 {
		t.Fatalf("Find() = %#v, want one case-insensitive match", matches)
	}
	match := matches[0]
	if got := source[match.Start:match.End]; got != "İSTANBUL" {
		t.Fatalf("match range = %q, want original source spelling", got)
	}
	if got := match.Snippet; got != source {
		t.Fatalf("snippet = %q, want %q", got, source)
	}
}

func TestFoldOffsetShiftsStaySparseAfterWidthChangingRune(t *testing.T) {
	source := "İ" + strings.Repeat("a", 1<<20)
	folded, shifts := foldWithSourceOffsets(source)
	if len(shifts) != 1 {
		t.Fatalf("offset shift count = %d, want one for the width-changing rune", len(shifts))
	}
	if got, ok := sourceOffsetAt(folded, shifts, len(folded)); !ok || got != len(source) {
		t.Fatalf("mapped end offset = %d, %v; want %d, true", got, ok, len(source))
	}
}

func TestReplaceAllProducesOneSourceRevision(t *testing.T) {
	source := "cat and cat"
	matches := Find(source, "cat")
	got, count := ReplaceAll(source, matches, "dog")
	if got != "dog and dog" || count != 2 {
		t.Fatalf("ReplaceAll = %q, %d", got, count)
	}
}

func TestReplaceAllRejectsInvalidRangesWithoutPartialOutput(t *testing.T) {
	source := "abcdef"
	matches := []Match{{Start: 1, End: 3}, {Start: 2, End: 4}}
	got, count := ReplaceAll(source, matches, "x")
	if got != source || count != 0 {
		t.Fatalf("ReplaceAll invalid ranges = %q, %d", got, count)
	}
}

func TestSnippetParts(t *testing.T) {
	before, match, after := SnippetParts("First Muse note", "muse")
	if before != "First " || match != "Muse" || after != " note" {
		t.Fatalf("parts = %q, %q, %q", before, match, after)
	}
	before, match, after = SnippetParts("İSTANBUL is here", "istanbul")
	if before != "" || match != "İSTANBUL" || after != " is here" {
		t.Fatalf("Unicode parts = %q, %q, %q", before, match, after)
	}
	before, match, after = SnippetParts("Muse 是原生 Markdown 编辑器", "markdown")
	if before != "Muse 是原生 " || match != "Markdown" || after != " 编辑器" {
		t.Fatalf("mixed-language parts = %q, %q, %q", before, match, after)
	}
	if before, match, after = SnippetParts("没有命中的行", "xyz"); match != "" || before != "没有命中的行" || after != "" {
		t.Fatalf("unmatched snippet = %q, %q, %q", before, match, after)
	}
	if before, match, after = SnippetParts("aa bb aa", "aa"); before != "" || match != "aa" || after != " bb aa" {
		t.Fatalf("parts should split at first match only: %q, %q, %q", before, match, after)
	}
	if before, match, after = SnippetParts("任意内容", "  "); match != "" || before != "任意内容" || after != "" {
		t.Fatalf("blank query parts = %q, %q, %q", before, match, after)
	}
}
