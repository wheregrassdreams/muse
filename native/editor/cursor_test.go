package mdedit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestCursorOverTasksAndLinks checks the pointer's shape where it rests: a
// hand over a task's check box and over a link, an I-beam over plain text.
func TestCursorOverTasksAndLinks(t *testing.T) {
	h := newHarness(t, "- [ ] todo\n\n[link](https://example.com) text")

	task := h.e.leaves[0]
	tx := h.e.colX + h.e.lk.size*itemIndent - h.e.lk.size*0.9
	ty := task.y + h.e.lk.size*0.8
	h.tt.Move(tx, ty)
	h.frame()
	if got := h.tt.Cursor(); got != ui.CursorPointer {
		t.Fatalf("cursor over the check box = %v, want pointer", got)
	}

	para := blocks(h.e)[1]
	lx, ly := h.e.colX+20, para.y+10
	if h.e.linkAt(h.e.hitTest(lx, ly)) == "" {
		t.Fatal("the test's pointer is not on the link")
	}
	h.tt.Move(lx, ly)
	h.frame()
	if got := h.tt.Cursor(); got != ui.CursorPointer {
		t.Fatalf("cursor over the link = %v, want pointer", got)
	}

	h.tt.Move(h.e.colX+700, ly)
	h.frame()
	if got := h.tt.Cursor(); got != ui.CursorText {
		t.Fatalf("cursor over the text = %v, want text", got)
	}
}
