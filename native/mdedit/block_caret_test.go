package mdedit

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestModalBlockCaret(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		h := newHarness(t, "a中\n")
		h.e.SetKeymap(mode)
		if !h.e.blockCaret() {
			t.Fatal("no block in normal")
		}
		h.caretAt(0)
		r, _ := h.e.caretRect(0)
		latin := h.e.blockCaretRect(r)
		h.caretAt(1)
		r, _ = h.e.caretRect(1)
		cjk := h.e.blockCaretRect(r)
		if latin.W <= 1.75 || cjk.W <= latin.W {
			t.Fatalf("wrong glyph widths: %v %v", latin, cjk)
		}
		h.shot(mode + "-block-caret")
		h.e.modalText("v")
		h.e.modalText("l")
		h.shot(mode + "-selection-block-caret")
		h.e.modalKey(ui.KeyEscape, 0)
		h.e.modalText("i")
		if h.e.blockCaret() {
			t.Fatal("insert must have bar caret")
		}
	}
}

func TestBlockCaretAtEndAndEmptyLine(t *testing.T) {
	h := newHarness(t, "abc\n\n")
	h.e.SetKeymap("vim")
	for _, at := range []int{3, 4, 5} {
		h.caretAt(at)
		r, ok := h.e.caretRect(at)
		if !ok {
			t.Fatalf("no caret at %d", at)
		}
		r = h.e.blockCaretRect(r)
		if r.W <= 1.75 || r.H <= 0 {
			t.Fatalf("invalid block at %d: %+v", at, r)
		}
	}
}
