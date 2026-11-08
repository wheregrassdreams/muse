package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestAnObjectIsOneCharacter: a picture, a formula, an embed is one unit of
// the text — the caret stands on either side of it and nowhere inside, the
// arrows cross it in one press, Backspace and Delete take it whole, and a
// click takes it whole and opens its source. The model is CodeMirror's
// atomic range and ProseMirror's atom node (docs/editor-model.md §6).
func TestAnObjectIsOneCharacter(t *testing.T) {
	const src = "前 ![图](a.png) 后"
	const from, to = 4, 17

	inside := func(off int) bool { return off > from && off < to }

	h := newHarness(t, src)
	// No key buts the caret inside: not the arrows, not a jump from far
	// away, not the ends of the line.
	for _, key := range []ui.Key{ui.KeyRight, ui.KeyLeft, ui.KeyUp, ui.KeyDown, ui.KeyEnd, ui.KeyHome} {
		h.caretAt(from - 1)
		h.key(0, key)
		if inside(h.e.caret) {
			t.Fatalf("%v took the caret to %d, inside the object %d..%d", key, h.e.caret, from, to)
		}
	}
	// An offset the display has no place for still draws a caret, at the
	// nearest place it does have.
	if _, ok := h.e.caretRect(from + 3); !ok {
		t.Fatal("no caret inside the object, though one must be drawn somewhere")
	}
	// Left and right cross it in one press, and never open it.
	h.caretAt(from - 1)
	h.key(0, ui.KeyRight)
	if h.e.caret != from {
		t.Fatalf("right = %d, want the object's start %d", h.e.caret, from)
	}
	h.key(0, ui.KeyRight)
	if h.e.caret != to {
		t.Fatalf("right = %d, want the object's end %d", h.e.caret, to)
	}
	if h.shows("![") {
		t.Fatal("crossing the object opened its source")
	}
	h.key(0, ui.KeyLeft)
	if h.e.caret != from {
		t.Fatalf("left = %d, want the object's start %d", h.e.caret, from)
	}

	// Backspace and Delete take it whole, one step each.
	for _, c := range []struct {
		name string
		key  ui.Key
		at   int
	}{{"backspace", ui.KeyBackspace, to}, {"delete", ui.KeyDelete, from}} {
		h := newHarness(t, src)
		h.caretAt(c.at)
		h.key(0, c.key)
		if got := h.e.Source(); got != "前  后" {
			t.Fatalf("%s gave %q, want the object gone whole", c.name, got)
		}
		h.key(ui.Cmd, ui.KeyZ)
		if got := h.e.Source(); got != src {
			t.Fatalf("%s: undo gave %q", c.name, got)
		}
	}

	// A click takes it whole and opens the source; typing replaces it, as
	// typing over a selected letter does.
	h = newHarness(t, src)
	h.caretAt(0)
	l := h.e.leaves[0]
	at := h.e.colX + l.view.tx
	for _, pc := range l.view.disp.pieces {
		if pc.obj != nil {
			at += float32(pc.d0) * h.e.lk.size * 0.6
		}
	}
	h.tt.ClickAt(h.e.colX+22, l.y+10)
	h.frame()
	if h.e.anchor != from || h.e.caret != to {
		t.Fatalf("the click selected %d..%d, want %d..%d", h.e.anchor, h.e.caret, from, to)
	}
	if !h.shows("![") {
		t.Fatal("the selection did not open the object's source")
	}
	h.typ("x")
	if got := h.e.Source(); got != "前 x 后" {
		t.Fatalf("typing over the object gave %q", got)
	}
}

// TestABlockImageIsOneCharacter: an image alone in its paragraph is the same
// unit, from anywhere on its line.
func TestABlockImageIsOneCharacter(t *testing.T) {
	h := newHarness(t, "![图](a.png)")
	if h.shows("![") {
		t.Fatal("the picture shows its source with the caret away")
	}
	h.caretAt(0)
	if h.shows("![") {
		t.Fatal("the picture shows its source with the caret at its start")
	}
	l := h.e.leaves[0]
	h.tt.ClickAt(h.e.colX+20, l.y+l.h/2)
	h.frame()
	if h.e.anchor != 0 || h.e.caret != len(h.e.Source()) {
		t.Fatalf("the click selected %d..%d, want the whole picture", h.e.anchor, h.e.caret)
	}
	h.key(0, ui.KeyBackspace)
	if got := h.e.Source(); strings.TrimSpace(got) != "" {
		t.Fatalf("Backspace left %q", got)
	}
}

// TestAnInlineFormulaIsOneCharacter: a formula is an object too.
func TestAnInlineFormulaIsOneCharacter(t *testing.T) {
	h := newHarness(t, "前 $x^2$ 后")
	if strings.Contains(h.e.leaves[0].view.disp.text, "x") {
		t.Skip("the formula did not typeset; nothing to be atomic about")
	}
	h.caretAt(3)
	h.key(0, ui.KeyRight)
	if h.e.caret != 4 {
		t.Fatalf("right = %d, want the formula's start 4", h.e.caret)
	}
	h.key(0, ui.KeyRight)
	if h.e.caret != 9 {
		t.Fatalf("right = %d, want the formula's end 9", h.e.caret)
	}
	if h.shows("$") {
		t.Fatal("crossing the formula opened its source")
	}
}
