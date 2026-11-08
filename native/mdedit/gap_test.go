package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestCommentLinesAreReachableByClick: the lines of a comment are blank in
// the parse and no leaf covers them, so a click there has to find the
// comment itself — it shows once the caret is in it, and only then.
func TestCommentLinesAreReachableByClick(t *testing.T) {
	const src = "before\n\n%% hidden note %%\n\nafter"
	// The line right below the paragraph is the blank line between them.
	h := newHarness(t, src)
	if h.tt.ClickAt(h.e.colX+40, h.e.leaves[0].y+42); true {
		h.frame()
		if h.e.caret != 7 {
			t.Fatalf("the blank line above the comment takes the caret at %d, want 7", h.e.caret)
		}
	}
	// The comment's own line takes the caret into the comment.
	h = newHarness(t, src)
	h.tt.ClickAt(h.e.colX+40, h.e.leaves[0].y+70)
	h.frame()
	if _, in := commentAt(h.e.comments, h.e.caret); !in {
		t.Fatalf("the click on the comment's line put the caret at %d, outside the comment", h.e.caret)
	}
	// And it can be typed in, as any other line.
	h.caretAt(h.e.comments[0].from + 3)
	h.typ("x")
	if got := h.e.Source(); !strings.Contains(got, "%% xhidden note %%") {
		t.Fatalf("typing in the comment gives %q", got)
	}
}

func TestArrowStopsOnBlankLines(t *testing.T) {
	h := newHarness(t, "a\n\n\nb")
	h.caretAt(0)
	h.key(0, ui.KeyDown)
	if h.e.caret != 2 {
		t.Fatalf("Down from the paragraph lands at %d, want the first blank line (2)", h.e.caret)
	}
	h.key(0, ui.KeyDown)
	if h.e.caret != 3 {
		t.Fatalf("the next Down lands at %d, want the second blank line (3)", h.e.caret)
	}
	h.key(0, ui.KeyDown)
	if h.e.caret != 4 {
		t.Fatalf("the next Down lands at %d, want the paragraph (4)", h.e.caret)
	}
	h.key(0, ui.KeyUp)
	if h.e.caret != 3 {
		t.Fatalf("Up lands at %d, want the last blank line (3)", h.e.caret)
	}
}
