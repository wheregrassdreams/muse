package mdedit

import "testing"

// blankY returns the y of the k-th blank line between the first two leaves.
func blankY(t *testing.T, e *Editor, k int) float32 {
	t.Helper()
	e.layoutIfNeeded()
	first := e.leaves[0]
	return first.y + first.h + e.lk.size*e.lk.lh*(float32(k)+0.5)
}

func TestGapPhantomFindsTheBlankLine(t *testing.T) {
	h := newHarness(t, "a\n\n\nb")
	if at, ok := h.e.gapPhantomAt(blankY(t, h.e, 0)); !ok || at != 2 {
		t.Fatalf("first blank line: at=%d ok=%v", at, ok)
	}
	if at, ok := h.e.gapPhantomAt(blankY(t, h.e, 1)); !ok || at != 3 {
		t.Fatalf("second blank line: at=%d ok=%v", at, ok)
	}
	// The middle of a paragraph is not a gap.
	first := h.e.leaves[0]
	if _, ok := h.e.gapPhantomAt(first.y + first.h/2); ok {
		t.Fatal("a paragraph's middle reads as a gap")
	}
	// The single blank line between two paragraphs is a line to sit on too.
	h2 := newHarness(t, "a\n\nb")
	if at, ok := h2.e.gapPhantomAt(blankY(t, h2.e, 0)); !ok || at != 2 {
		t.Fatalf("the paragraph break: at=%d ok=%v", at, ok)
	}
}

func TestTypingInAGap(t *testing.T) {
	h := newHarness(t, "a\n\n\nb")
	at, ok := h.e.gapPhantomAt(blankY(t, h.e, 0))
	if !ok {
		t.Fatal("no gap")
	}
	h.e.setPhantom(&phantom{at: at})
	h.frame()
	h.typ("x")
	if got := h.e.Source(); got != "a\nx\n\nb" {
		t.Fatalf("typing in a blank line gives %q", got)
	}
}
