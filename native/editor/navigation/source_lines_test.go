package navigation

import "testing"

func TestLineRangeUsesLFOnlyAndKeepsByteOffsets(t *testing.T) {
	source := "甲\r\n\t乙\n"
	cases := []struct {
		name   string
		offset int
		want   Range
	}{
		{"start", 0, Range{Start: 0, End: len("甲\r")}},
		{"inside UTF8 line", len("甲"), Range{Start: 0, End: len("甲\r")}},
		{"newline byte", len("甲\r"), Range{Start: 0, End: len("甲\r")}},
		{"next line", len("甲\r\n"), Range{Start: len("甲\r\n"), End: len("甲\r\n\t乙")}},
		{"trailing empty line", len(source), Range{Start: len(source), End: len(source)}},
		{"negative clamps to start", -10, Range{Start: 0, End: len("甲\r")}},
		{"past end clamps", len(source) + 10, Range{Start: len(source), End: len(source)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LineRange(source, tc.offset); got != tc.want {
				t.Fatalf("LineRange(%d) = %+v, want %+v", tc.offset, got, tc.want)
			}
		})
	}
	if got := LineEnd(source, len("甲\r\n")); got != len("甲\r\n\t乙") {
		t.Fatalf("LineEnd() = %d", got)
	}
}

func TestLineStartSkipsOnlyLeadingSpacesAndTabsWithinLine(t *testing.T) {
	source := " \t 甲\n \t\n\u00a0x"
	if got, want := LineStart(source, 2, false), 0; got != want {
		t.Fatalf("LineStart without trim = %d, want %d", got, want)
	}
	if got, want := LineStart(source, 2, true), len(" \t "); got != want {
		t.Fatalf("LineStart first-nonspace = %d, want %d", got, want)
	}
	blankStart := len(" \t甲\n")
	if got, want := LineStart(source, blankStart, true), blankStart+len(" \t"); got != want {
		t.Fatalf("blank-line first-nonspace = %d, want %d", got, want)
	}
	nbspStart := len(" \t甲\n \t\n")
	if got := LineStart(source, nbspStart, true); got != nbspStart {
		t.Fatalf("LineStart skipped non-ASCII whitespace: got %d want %d", got, nbspStart)
	}
}
