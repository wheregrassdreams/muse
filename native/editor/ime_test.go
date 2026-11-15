package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestComposeOnNewLineStaysOnNewLine: Enter at the end of a heading puts the
// caret in an empty paragraph that starts where the heading ends. What the
// input method composes there belongs to that paragraph and only to it: the
// heading must not grow a copy of it while it is being composed.
func TestComposeOnNewLineStaysOnNewLine(t *testing.T) {
	h := newHarness(t, "# 123qweq we")
	h.caretAt(len("# 123qweq we"))
	h.key(0, ui.KeyEnter)
	if l := h.e.leafAt(h.e.caret); l == nil || l.kind != kBlank {
		t.Fatalf("Enter at the end of the heading did not open a line: %+v", l)
	}
	h.e.SetMarkedText(nil, "q.we", ui.TextInputRange{Start: 4, End: 4})
	h.frame()

	if len(h.e.leaves) != 2 {
		t.Fatalf("leaves = %d, want the heading and the paragraph after it", len(h.e.leaves))
	}
	head, line := h.e.leaves[0], h.e.leaves[1]
	if head.kind != kHeading || line.kind != kBlank {
		t.Fatalf("leaves = %d, %d; want a heading and the line below it", head.kind, line.kind)
	}
	if head.view == nil || line.view == nil {
		t.Fatal("the heading or the new paragraph has no view")
	}
	if got := head.view.disp.text; strings.Contains(got, "q.we") {
		t.Fatalf("the composition shows in the heading: %q", got)
	}
	if got := line.view.disp.text; !strings.Contains(got, "q.we") {
		t.Fatalf("the composition is not in the new paragraph: %q", got)
	}
	n := 0
	for _, l := range h.e.leaves {
		if l.view == nil {
			continue
		}
		for _, p := range l.view.disp.pieces {
			if p.kind == pDeco && p.text == "q.we" {
				n++
			}
		}
	}
	if n != 1 {
		t.Fatalf("the composition is drawn %d times, want once", n)
	}
	h.shot("compose-new-line")

	// The commit goes into the paragraph, after the heading's own line.
	h.e.ReplaceText(nil, "q.we")
	h.frame()
	h.want("# 123qweq we\nq.we")
	h.shot("compose-new-line-committed")
}
