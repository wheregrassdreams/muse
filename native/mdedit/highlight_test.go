package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestHighlightShowsItsText(t *testing.T) {
	if got := shown(t, "前 ==重点== 后\n"); len(got) != 1 || got[0] != "前 «mk:重点» 后" {
		t.Fatalf("shows %q", got)
	}
	// Not inside a word of Latin letters and digits.
	for _, src := range []string{"a==b && c==d\n", "x==2==y\n"} {
		if got := shown(t, src); len(got) != 1 || got[0] != strings.TrimSuffix(src, "\n") {
			t.Errorf("%q shows %q", src, got)
		}
	}
}

func TestHighlightRevealsItsMarksAtTheCaret(t *testing.T) {
	h := newHarness(t, "one ==two== three")
	h.caretAt(0)
	if h.shows("==") {
		t.Fatal("the marks show away from the caret")
	}
	h.caretAt(7)
	if !h.shows("==two==") {
		t.Fatalf("the marks are hidden at the caret: %q", h.e.leaves[0].view.disp.text)
	}
}

func TestToggleHighlightWrapsAndUnwraps(t *testing.T) {
	h := newHarness(t, "one two three")
	h.e.anchor, h.e.caret = 4, 7
	h.frame()
	h.key(ui.Cmd|ui.Shift, ui.KeyH)
	h.want("one ==two== three")
	if h.e.anchor != 6 || h.e.caret != 9 {
		t.Fatalf("selection %d..%d, want 6..9", h.e.anchor, h.e.caret)
	}
	h.key(ui.Cmd|ui.Shift, ui.KeyH)
	h.want("one two three")
	if h.e.anchor != 4 || h.e.caret != 7 {
		t.Fatalf("selection %d..%d, want 4..7", h.e.anchor, h.e.caret)
	}

	// A caret in a word marks the word.
	h = newHarness(t, "one two three")
	h.caretAt(5)
	h.key(ui.Cmd|ui.Shift, ui.KeyH)
	h.want("one ==two== three")
}

func TestHighlightIsNotInThePlainText(t *testing.T) {
	e := shownEditor(t, "前 ==重点== 后\n")
	if got := e.plainText(e.p.doc.Inlines(e.leaves[0].id)); got != "前 重点 后" {
		t.Fatalf("plain text %q", got)
	}
}
