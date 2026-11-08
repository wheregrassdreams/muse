package mdedit

import (
	"strings"
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

// blocks returns the leaves that hold text: the blocks of the document,
// without the blank lines between them.
func blocks(e *Editor) []*leaf {
	var out []*leaf
	for _, l := range e.leaves {
		if l.kind != kBlank {
			out = append(out, l)
		}
	}
	return out
}

// lineH returns the height of a line of the editor's text.
func lineH(e *Editor) float32 { return e.lk.size * e.lk.lh }

func TestBlankLinesTakeRoom(t *testing.T) {
	// Two blank lines take one line more than one does: the layout's lines
	// and the document's lines agree, so Enter on an empty line shows.
	one := newHarness(t, "a\n\nb").e
	two := newHarness(t, "a\n\n\nb").e
	if n := len(one.leaves); n != 3 {
		t.Fatalf("%d leaves, want a paragraph, a blank line and a paragraph", n)
	}
	if n := len(two.leaves); n != 4 {
		t.Fatalf("%d leaves, want two blank lines of their own", n)
	}
	y1, y2 := blocks(one)[1].y, blocks(two)[1].y
	if d := y2 - y1; d < lineH(one)*0.9 || d > lineH(one)*1.1 {
		t.Fatalf("a second blank line moves the paragraph by %v, want about a line", d)
	}
	// Every line of the document is a line of the layout.
	for i, l := range one.leaves {
		if want := one.leaves[0].y + float32(i)*lineH(one); l.y != want {
			t.Fatalf("leaf %d at y=%.1f, want %.1f", i, l.y, want)
		}
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

func TestBackspacePeelsOneBlankLine(t *testing.T) {
	h := newHarness(t, "a\n\n\nb")
	h.caretAt(4)
	h.key(0, ui.KeyBackspace)
	if got := h.e.Source(); got != "a\n\nb" {
		t.Fatalf("one Backspace leaves %q, want one blank line gone", got)
	}
	// The blank lines go one at a time: the second leaves the paragraphs
	// with a single break between them, the third joins them.
	h.key(0, ui.KeyBackspace)
	if got := h.e.Source(); got != "a\nb" {
		t.Fatalf("the next Backspace leaves %q", got)
	}
	h.key(0, ui.KeyBackspace)
	if got := h.e.Source(); got != "ab" {
		t.Fatalf("the last Backspace leaves %q", got)
	}
}

func TestLeadingBlankLinesTakeRoom(t *testing.T) {
	one := blocks(newHarness(t, "\n正文").e)
	two := blocks(newHarness(t, "\n\n正文").e)
	if len(one) == 0 || len(two) == 0 {
		t.Fatalf("no leaves: %v %v", len(one), len(two))
	}
	if d := two[0].y - one[0].y; d < 20 {
		t.Fatalf("a leading blank line moves the text by %v, want a line", d)
	}
	// A leading blank line is a line the caret can stand on.
	h := newHarness(t, "\n正文")
	if l := h.e.leaves[0]; l.kind != kBlank || l.start != 0 {
		t.Fatalf("first leaf kind %d at %d, want the blank line at 0", l.kind, l.start)
	}
}

// TestTypingOnABlankLine: the caret on an empty line of the source writes
// there, and the line stops being empty.
func TestTypingOnABlankLine(t *testing.T) {
	h := newHarness(t, "a\n\nb")
	h.caretAt(2)
	h.typ("x")
	// The three lines are one paragraph now, as they read in the source:
	// the blank line was a line between them, not a block of its own.
	h.want("a\nx\nb")
	if h.e.caret != 3 {
		t.Fatalf("the caret is at %d, want 3", h.e.caret)
	}
	if n := len(h.e.leaves); n != 1 {
		t.Fatalf("%d leaves, want the one paragraph", n)
	}
}

// TestClickOnABlankLinePuts the caret on it, as a click on any other line.
func TestClickOnABlankLinePuts(t *testing.T) {
	h := newHarness(t, "a\n\nb")
	blank := h.e.leaves[1]
	if blank.kind != kBlank {
		t.Fatalf("leaf 1 is kind %d, want the blank line", blank.kind)
	}
	h.tt.ClickAt(h.e.colX+20, blank.y+blank.h/2)
	h.frame()
	if h.e.caret != blank.start {
		t.Fatalf("the caret is at %d, want the blank line at %d", h.e.caret, blank.start)
	}
}

// TestSelectAcrossABlankLine deletes what it covers, blank lines included.
func TestSelectAcrossABlankLine(t *testing.T) {
	h := newHarness(t, "a\n\nb")
	h.e.Select(1, 3)
	h.frame()
	if got := h.e.SelectedText(); got != "\n\n" {
		t.Fatalf("the selection holds %q", got)
	}
	h.key(0, ui.KeyBackspace)
	h.want("ab")
}

// TestUndoOnABlankLine puts back the breaks a delete took.
func TestUndoOnABlankLine(t *testing.T) {
	h := newHarness(t, "a\n\nb")
	h.caretAt(2)
	h.key(0, ui.KeyBackspace)
	h.want("a\nb")
	h.key(ui.Cmd, ui.KeyZ)
	h.want("a\n\nb")
}

// TestCaretRectOnABlankLine draws the caret on the line, not on the block
// above it — the input method puts what it composes where the caret is.
func TestCaretRectOnABlankLine(t *testing.T) {
	h := newHarness(t, "a\n\nb")
	blank := h.e.leaves[1]
	r, ok := h.e.caretRect(blank.start)
	if !ok {
		t.Fatal("no caret on the blank line")
	}
	if r.Y < blank.y || r.Y > blank.y+blank.h {
		t.Fatalf("the caret is at y=%.1f, outside its line [%.1f, %.1f]", r.Y, blank.y, blank.y+blank.h)
	}
}

// TestComposeOnABlankLine shows what the input method composes on the line
// it is being composed on: a blank line has no text to draw it in, so the
// layout gives it a view while it holds the composition.
func TestComposeOnABlankLine(t *testing.T) {
	h := newHarness(t, "a\n\nb")
	h.caretAt(2)
	h.e.SetMarkedText(nil, "zhong", ui.TextInputRange{Start: 5, End: 5})
	h.frame()
	blank := h.e.leaves[1]
	if blank.kind != kBlank {
		t.Fatalf("leaf 1 is kind %d, want the blank line", blank.kind)
	}
	if blank.view == nil || !strings.Contains(blank.view.disp.text, "zhong") {
		t.Fatalf("the composition is not on the blank line: %+v", blank.view)
	}
	h.e.ReplaceText(nil, "中文")
	h.frame()
	h.want("a\n中文\nb")
}
