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
	if h.e.Source() != "a\n\n\n" {
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

// TestSelectionPaintsAcrossABlankLine: a selection covers the blank lines it
// spans, not only the text on either side — a blank line has no view to
// draw text in, so the paint draws the band itself.
func TestSelectionPaintsAcrossABlankLine(t *testing.T) {
	h := newHarness(t, "a\n\nb")
	blank := h.e.leaves[1]
	x, y := int(h.e.colX)+200, int(blank.y+blank.h/2)
	before := h.tt.Image().RGBAAt(x, y)
	h.e.Select(1, 3)
	h.frame()
	after := h.tt.Image().RGBAAt(x, y)
	if before == after {
		t.Fatalf("the blank line at %.0f is not painted as selected (pixel %v)", blank.y, before)
	}
}

// TestQuotationBlankLineTakesText: the empty line of a quotation carries
// its `>`, and the caret belongs after it — typing there keeps the line in
// the quotation instead of putting the text before the marker.
func TestQuotationBlankLineTakesText(t *testing.T) {
	h := newHarness(t, "> a\n>\n> b")
	blank := h.e.leaves[1]
	if blank.kind != kBlank || blank.start != 5 || len(blank.conts) != 1 {
		t.Fatalf("leaf 1 is kind %d at %d with %d containers, want the `>` line at 5",
			blank.kind, blank.start, len(blank.conts))
	}
	h.tt.ClickAt(h.e.colX+20, blank.y+blank.h/2)
	h.frame()
	if h.e.caret != 5 {
		t.Fatalf("the click put the caret at %d, want 5", h.e.caret)
	}
	h.typ("x")
	h.want("> a\n>x\n> b")
	for _, line := range strings.Split(h.e.Source(), "\n") {
		if !strings.HasPrefix(line, ">") {
			t.Fatalf("the quotation is broken: %q", h.e.Source())
		}
	}
}

// TestQuotationBlankLinesTakeArrows: every empty line of a quotation is a
// line the arrows stop on.
func TestQuotationBlankLinesTakeArrows(t *testing.T) {
	h := newHarness(t, "> a\n>\n>\n> b")
	h.caretAt(3)
	h.key(0, ui.KeyDown)
	if h.e.caret != 5 {
		t.Fatalf("the first Down lands at %d, want the first empty line (5)", h.e.caret)
	}
	h.key(0, ui.KeyDown)
	if h.e.caret != 7 {
		t.Fatalf("the second Down lands at %d, want the second empty line (7)", h.e.caret)
	}
	h.key(0, ui.KeyDown)
	if h.e.caret < 10 {
		t.Fatalf("the third Down lands at %d, want the paragraph at 10 or after", h.e.caret)
	}
}

// TestClickOnEachQuotationBlankLine: a click lands on the line it is on,
// not one line off, however many empty lines the quotation has.
func TestClickOnEachQuotationBlankLine(t *testing.T) {
	h := newHarness(t, "> a\n>\n>\n> b")
	first := h.e.leaves[0]
	for _, c := range []struct {
		dy   float32
		want int
	}{{42, 5}, {70, 7}, {98, 10}} {
		h.tt.ClickAt(h.e.colX+20, first.y+c.dy)
		h.frame()
		if h.e.caret != c.want {
			t.Fatalf("a click %.0f below the first line put the caret at %d, want %d",
				c.dy, h.e.caret, c.want)
		}
	}
}

// TestBackspacePeelsOneQuotationBlankLine: Backspace on an empty line of a
// quotation takes the line — its break and its marker — and nothing else.
func TestBackspacePeelsOneQuotationBlankLine(t *testing.T) {
	h := newHarness(t, "> a\n>\n>\n> b")
	h.caretAt(7)
	h.key(0, ui.KeyBackspace)
	h.want("> a\n>\n> b")
	h.key(0, ui.KeyBackspace)
	h.want("> a\n> b")
}

// TestDeletePeelsOneQuotationBlankLine: Delete at the end of a quotation's
// line takes the empty line below it, one line at a time.
func TestDeletePeelsOneQuotationBlankLine(t *testing.T) {
	h := newHarness(t, "> a\n>\n>\n> b")
	h.caretAt(3)
	h.key(0, ui.KeyDelete)
	h.want("> a\n>\n> b")
	h.key(0, ui.KeyDelete)
	h.want("> a\n> b")
}

// TestEnterOnAQuotationBlankLine: Enter on an empty line of a quotation
// opens another line of it, marker and all.
func TestEnterOnAQuotationBlankLine(t *testing.T) {
	h := newHarness(t, "> a\n>\n> b")
	h.caretAt(5)
	// An empty line of a quotation is how the quotation ends: Enter takes
	// its marker, and the line after it is plain.
	h.key(0, ui.KeyEnter)
	if got := h.e.Source(); got != "> a\n\n> b" {
		t.Fatalf("Enter on the empty line leaves %q, want the quotation ended", got)
	}
}

// TestNestedQuotationBlankLine: a `> >` line is inside both quotations,
// and sits at the indent of the text above and below it.
func TestNestedQuotationBlankLine(t *testing.T) {
	h := newHarness(t, "> > a\n> >\n> > b")
	blank, above := h.e.leaves[1], h.e.leaves[0]
	if len(blank.conts) != 2 {
		t.Fatalf("the empty line is in %d containers, want 2", len(blank.conts))
	}
	if blank.x != above.x {
		t.Fatalf("the empty line starts at %.1f, want the text's %.1f", blank.x, above.x)
	}
	h.caretAt(blank.start)
	h.typ("x")
	h.want("> > a\n> >x\n> > b")
}

// TestBlankLineInACallout: the empty line between two paragraphs of a
// callout is the callout's, and typing there stays in it.
func TestBlankLineInACallout(t *testing.T) {
	h := newHarness(t, "> [!NOTE]\n> 一\n>\n> 二")
	var blank *leaf
	for _, l := range h.e.leaves {
		if l.kind == kBlank {
			blank = l
		}
	}
	if blank == nil || len(blank.conts) != 1 {
		t.Fatalf("no empty line of the callout: %+v", blank)
	}
	h.tt.ClickAt(h.e.colX+40, blank.y+blank.h/2)
	h.frame()
	h.typ("x")
	h.want("> [!NOTE]\n> 一\n>x\n> 二")
}

// TestBlankLineInAListItemIsTheDocument's: an empty line inside a list item
// carries no marker, so it is a line at the start of the column, and typing
// there writes there.
func TestBlankLineInAListItemIsTheDocuments(t *testing.T) {
	h := newHarness(t, "- a\n\n  more")
	blank := blocks(h.e)[0]
	_ = blank
	var b *leaf
	for _, l := range h.e.leaves {
		if l.kind == kBlank {
			b = l
		}
	}
	if b == nil || len(b.conts) != 0 || b.start != 4 {
		t.Fatalf("the empty line is %+v, want a line of its own at 4", b)
	}
	h.caretAt(4)
	h.typ("x")
	h.want("- a\nx\n  more")
}

// TestQuotationInAnItemKeepsItsBlankLine: a quotation inside a list item is
// still a quotation, and its empty `>` line sits at the item's column.
func TestQuotationInAnItemKeepsItsBlankLine(t *testing.T) {
	h := newHarness(t, "- > a\n  >\n  > b")
	blank, above := h.e.leaves[1], h.e.leaves[0]
	if len(blank.conts) != 2 || blank.x != above.x {
		t.Fatalf("the empty line is in %d containers at x=%.1f, want the item's column %.1f",
			len(blank.conts), blank.x, above.x)
	}
	h.caretAt(blank.start)
	h.typ("x")
	h.want("- > a\n  >x\n  > b")
}
