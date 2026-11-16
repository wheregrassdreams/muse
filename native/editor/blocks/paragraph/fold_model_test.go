package paragraph

import "testing"

func TestHeadingContentStartAdvancesPastSourceLineEnding(t *testing.T) {
	for _, tc := range []struct {
		name     string
		source   string
		blockEnd int
		want     int
	}{
		{"LF", "# title\nbody", len("# title"), len("# title\n")},
		{"CRLF", "# title\r\nbody", len("# title"), len("# title\r\n")},
		{"lone CR", "# title\rbody", len("# title"), len("# title\r")},
		{"EOF", "# title", len("# title"), len("# title")},
		{"clamped negative", "body", -1, 0},
		{"clamped past EOF", "body", 99, len("body")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := HeadingContentStart(tc.source, tc.blockEnd); got != tc.want {
				t.Fatalf("HeadingContentStart() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestFoldSectionHidesOnlyFoldedContentOffsets(t *testing.T) {
	section := FoldSection{From: 10, To: 20, HasContent: true}
	for _, tc := range []struct {
		offset int
		folded bool
		want   bool
	}{
		{9, true, false},
		{10, true, true},
		{19, true, true},
		{20, true, false},
		{15, false, false},
	} {
		if got := section.Hides(tc.folded, tc.offset); got != tc.want {
			t.Errorf("Hides(%v, %d) = %v, want %v", tc.folded, tc.offset, got, tc.want)
		}
	}
}
