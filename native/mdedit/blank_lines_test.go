package mdedit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// leafYs returns the top of every leaf, in the last layout.
func leafYs(e *Editor) []float32 {
	var out []float32
	for _, l := range e.leaves {
		out = append(out, float32(l.y))
	}
	return out
}

func TestBlankLinesTakeRoom(t *testing.T) {
	// Two blank lines take one line more than one does: the layout's lines
	// and the document's lines agree, so Enter on an empty line shows.
	one := leafYs(newHarness(t, "a\n\nb").e)
	two := leafYs(newHarness(t, "a\n\n\nb").e)
	if len(one) != 2 || len(two) != 2 {
		t.Fatalf("leaves %v %v", one, two)
	}
	if d := two[1] - one[1]; d < 20 || d > 40 {
		t.Fatalf("a second blank line moves the paragraph by %v, want about a line", d)
	}
}

func TestEnterOnABlankLineAddsAVisibleLine(t *testing.T) {
	h := newHarness(t, "a\n\nb")
	h.caretAt(2)
	before := leafYs(h.e)
	h.key(0, ui.KeyEnter)
	after := leafYs(h.e)
	if got := h.e.Source(); got != "a\n\n\nb" {
		t.Fatalf("source %q", got)
	}
	// The paragraph below moved down by a line.
	last := func(ys []float32) float32 { return ys[len(ys)-1] }
	if d := last(after) - last(before); d < 20 {
		t.Fatalf("the paragraph moved %v, want a line", d)
	}
}

func TestEnterAtTheEndAddsAVisibleLine(t *testing.T) {
	h := newHarness(t, "a")
	h.caretAt(1)
	h.key(0, ui.KeyEnter)
	first := leafYs(h.e)
	h.key(0, ui.KeyEnter)
	second := leafYs(h.e)
	h.key(0, ui.KeyEnter)
	third := leafYs(h.e)
	if h.e.Source() != "a\n\n" {
		t.Fatalf("source %q", h.e.Source())
	}
	last := func(ys []float32) float32 { return ys[len(ys)-1] }
	if d := last(second) - last(first); d < 20 {
		t.Fatalf("the second Enter moved %v, want a line", d)
	}
	if d := last(third) - last(second); d < 20 {
		t.Fatalf("the third Enter moved %v, want a line", d)
	}
}
