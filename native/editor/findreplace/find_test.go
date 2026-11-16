package findreplace

import "testing"

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
}
