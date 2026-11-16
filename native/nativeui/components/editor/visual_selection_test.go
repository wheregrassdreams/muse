package editor

import (
	"strings"
	"testing"
)

func TestVimVisualUsesWholeGraphemeEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name, src, start, motion, want string
	}{
		{"combining single", "Ae\u0301B", "e\u0301", "", "e\u0301"},
		{"combining left", "Ae\u0301B", "e\u0301", "h", "Ae\u0301"},
		{"combining right", "Ae\u0301B", "e\u0301", "l", "e\u0301B"},
		{"ZWJ emoji single", "a👩‍💻b", "👩‍💻", "", "👩‍💻"},
		{"ZWJ emoji right", "a👩‍💻b", "👩‍💻", "l", "👩‍💻b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, tc.src)
			h.e.SetKeymap("vim")
			at := strings.Index(tc.src, tc.start)
			h.e.Select(at, at)
			sendVim(h, "v"+tc.motion)
			if got := h.e.SelectedText(); got != tc.want {
				t.Fatalf("selection = %q, want grapheme-complete %q", got, tc.want)
			}
			r := h.e.SelectionRange()
			if got := h.e.Source()[r.Start:r.End]; got != tc.want {
				t.Fatalf("native range = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVimVisualDollarExcludesLineBreak(t *testing.T) {
	h := newHarness(t, "abc\nnext")
	h.e.SetKeymap("vim")
	h.e.Select(0, 0)
	sendVim(h, "v$")
	if got := h.e.SelectedText(); got != "abc" {
		t.Fatalf("v$ selected %q, want line text without newline", got)
	}
}

func TestInclusiveSelectionCachesSourceGraphemesPerRevision(t *testing.T) {
	e := New(strings.Repeat("word 👩‍💻 e\u0301 ", 1200), Options{})
	e.selectionInclusive = true
	e.anchor, e.caret = 2, 2
	for range 100 {
		e.selection()
	}
	if e.graphemeBuilds != 1 {
		t.Fatalf("selection queries segmented source %d times in one revision", e.graphemeBuilds)
	}
	e.begin(stepEdit)
	e.replace(0, 0, "x")
	e.commit(3, 3)
	e.selectionInclusive = true
	e.selection()
	if e.graphemeBuilds != 2 {
		t.Fatalf("source revision did not rebuild grapheme index once: builds=%d", e.graphemeBuilds)
	}
}
