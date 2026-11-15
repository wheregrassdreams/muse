package document

import (
	"strings"
	"testing"
)

func TestMergeDisjointDocumentEdits(t *testing.T) {
	base := "---\ntags: []\n---\nfirst\nsecond\nthird\n"
	for _, tc := range []struct{ name, local, external, want string }{
		{"frontmatter and body", "---\ntags: [one]\n---\nfirst\nsecond\nthird\n", "---\ntags: []\n---\nfirst\nsecond changed\nthird\n", "---\ntags: [one]\n---\nfirst\nsecond changed\nthird\n"},
		{"independent body lines", "---\ntags: []\n---\nfirst changed\nsecond\nthird\n", "---\ntags: []\n---\nfirst\nsecond\nthird changed\n", "---\ntags: []\n---\nfirst changed\nsecond\nthird changed\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := MergeDisjointEdits(base, tc.local, tc.external)
			if !ok || got != tc.want {
				t.Fatalf("merge = (%q, %v), want (%q, true)", got, ok, tc.want)
			}
		})
	}
}

func TestMergeOverlappingDocumentEditsConflicts(t *testing.T) {
	base := "one\ntwo\nthree\n"
	for _, tc := range []struct{ local, external string }{
		{"ONE\ntwo\nthree\n", "uno\ntwo\nthree\n"},
		{"one\ntwo\nthree\nlocal\n", "one\ntwo\nthree\nexternal\n"},
	} {
		if _, ok := MergeDisjointEdits(base, tc.local, tc.external); ok {
			t.Fatalf("overlapping changes were merged: local=%q external=%q", tc.local, tc.external)
		}
	}
}

func TestLineDiffHunksRebuildShortInputs(t *testing.T) {
	sequences := []string{"", "a\n", "b\n", "a\na\n", "a\nb\n", "b\na\n", "b\nb\n", "a\nb\na\n"}
	for _, base := range sequences {
		for _, changed := range sequences {
			hunks, ok := lineDiffHunks(base, changed)
			if !ok {
				t.Fatalf("diff unexpectedly declined short input %q -> %q", base, changed)
			}
			lines := splitLines(base)
			var rebuilt strings.Builder
			cursor := 0
			for _, h := range hunks {
				rebuilt.WriteString(strings.Join(lines[cursor:h.start], ""))
				rebuilt.WriteString(h.text)
				cursor = h.end
			}
			rebuilt.WriteString(strings.Join(lines[cursor:], ""))
			if got := rebuilt.String(); got != changed {
				t.Fatalf("hunks rebuilt %q -> %q as %q (%+v)", base, changed, got, hunks)
			}
		}
	}
}
