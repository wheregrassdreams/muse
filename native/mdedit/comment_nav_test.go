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
	// The comment's lines, then the line after it — never back to the top.
	want := []int{3, 10, 13, 19}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Down visits %v, want %v", got, want)
	}
}
