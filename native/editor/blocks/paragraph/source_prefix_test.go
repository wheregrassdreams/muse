package paragraph

import "testing"

func TestCaretAfterLeadingIndent(t *testing.T) {
	for _, tc := range []struct {
		name         string
		source       string
		lineStart    int
		contentStart int
		caret        int
		want         bool
	}{
		{name: "after one space", source: "alpha\n beta", lineStart: 6, contentStart: 7, caret: 7, want: true},
		{name: "inside two spaces", source: "alpha\n  beta", lineStart: 6, contentStart: 8, caret: 7, want: true},
		{name: "before indentation", source: "alpha\n beta", lineStart: 6, contentStart: 7, caret: 6},
		{name: "container marker", source: "alpha\n> beta", lineStart: 6, contentStart: 8, caret: 8},
		{name: "ordinary content", source: "alpha\nbeta", lineStart: 6, contentStart: 6, caret: 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CaretAfterLeadingIndent(tc.source, tc.lineStart, tc.contentStart, tc.caret); got != tc.want {
				t.Fatalf("CaretAfterLeadingIndent() = %v, want %v", got, tc.want)
			}
		})
	}
}
