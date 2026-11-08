package mdedit

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDownThroughABlockCommentMovesOn(t *testing.T) {
	h := newHarness(t, "%%\n注释\n%%\n后文")
	h.caretAt(0)
	var got []int
	for range 4 {
		h.key(0, ui.KeyDown)
		got = append(got, h.e.caret)
	}
	// It moves down and never back to the top: the comment's lines are blank
	// lines in the file, so the caret walks them until it reaches the text.
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Fatalf("Down visits %v: it went back up", got)
		}
	}
	if got[len(got)-1] != 13 {
		t.Fatalf("Down ends at %d, want the text at 13 (%v)", got[len(got)-1], got)
	}
	_ = fmt.Sprint
}
