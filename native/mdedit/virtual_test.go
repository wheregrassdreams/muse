package mdedit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// proseSource returns n paragraphs, each a leaf of its own, so that a
// document of it is large enough to be laid out virtually.
func proseSource(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "Paragraph %d: the quick brown fox jumps over the lazy dog.\n\n", i)
	}
	return b.String()
}

// paragraphLeaf returns the leaf of "Paragraph n", the blank lines between
// paragraphs shifting the indexes.
func paragraphLeaf(t *testing.T, e *Editor, n int) *leaf {
	t.Helper()
	want := fmt.Sprintf("Paragraph %d:", n)
	for _, l := range e.leaves {
		if l.kind == kPara && strings.HasPrefix(e.src[l.start:], want) {
			return l
		}
	}
	t.Fatalf("no paragraph %d", n)
	return nil
}

// shapedLeaves returns how many leaves have a view: those the layout has
// shaped at some point.
func shapedLeaves(e *Editor) int {
	n := 0
	for _, l := range e.leaves {
		if !l.isLine() && l.view != nil {
			n++
		}
	}
	return n
}

// TestVirtualLayoutFirstFrame checks that a large document opened before
// the window showed anything does not shape all of its leaves: the first
// frame has no viewport yet, but a document of thousands of leaves already
// knows that most of them are far below the top.
func TestVirtualLayoutFirstFrame(t *testing.T) {
	ed := New(proseSource(1200), Options{FontSize: 16})
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			ed.View(c).Grow(1)
		})
	}, 1000, 700)
	tt.Frame()
	tt.Frame()

	if len(ed.leaves) < virtualLayoutMin {
		t.Fatalf("%d leaves, want at least %d", len(ed.leaves), virtualLayoutMin)
	}
	if n, total := shapedLeaves(ed), len(ed.leaves); n > total/4 {
		t.Fatalf("the first frame shaped %d of %d leaves, want only those near the top", n, total)
	}
	if ed.leaves[0].view == nil {
		t.Fatal("the first leaf is not shaped, though it is at the top of the document")
	}
	if l := blocks(ed)[len(blocks(ed))-1]; l.view != nil {
		t.Fatalf("the last leaf is shaped, though it is far below the viewport: %q", l.view.disp.text)
	}
}

// TestVirtualLayoutHeightChange checks that a taller window shapes the
// leaves its new viewport brings in. A window that only grows taller
// does not change the column's width, so nothing but the viewport itself
// asks for another layout.
func TestVirtualLayoutHeightChange(t *testing.T) {
	h := newHarness(t, "")
	h.e.SetSource(proseSource(1200))
	h.frame()
	mid := h.e.leaves[len(h.e.leaves)/2]
	h.e.ScrollToOffset(mid.start)
	for i := 0; i < 3; i++ {
		h.frame()
	}

	// A leaf below what the window shows now, but within the taller one.
	var target *leaf
	low := h.e.viewBottom
	for _, l := range h.e.leaves {
		if l.kind != kPara {
			continue
		}
		if l.y > low+40 && l.y < low+500 {
			target = l
			break
		}
	}
	if target == nil {
		t.Fatalf("no leaf between %.0f and %.0f below the viewport", low+40, low+500)
	}
	if target.view != nil {
		t.Fatalf("leaf at y=%.0f is already shaped, below the viewport ending at %.0f", target.y, low)
	}

	h.tt.SetSize(1000, 1400)
	h.frame()
	h.frame()

	if target.view == nil {
		t.Fatalf("leaf at y=%.0f unshaped after the window grew, though the viewport now ends at %.0f",
			target.y, h.e.viewBottom)
	}
	if !strings.Contains(target.view.disp.text, "Paragraph") {
		t.Fatalf("shaped leaf shows %q, want a paragraph", target.view.disp.text)
	}
}

// TestVirtualLayoutScroll checks that scrolling shapes the leaves the new
// viewport shows, without shaping the ones between the old and the new
// position.
func TestVirtualLayoutScroll(t *testing.T) {
	h := newHarness(t, "")
	h.e.SetSource(proseSource(1200))
	h.frame()

	target := paragraphLeaf(t, h.e, 700)
	at := 0
	for i, l := range h.e.leaves {
		if l == target {
			at = i
		}
	}
	if target.view != nil {
		t.Fatalf("leaf %d is shaped before scrolling to it", at)
	}
	h.e.ScrollToOffset(target.start)
	for i := 0; i < 4; i++ {
		h.frame()
	}

	if h.e.scroll.Y <= 0 {
		t.Fatalf("the scroll stayed at the top, want the target's offset")
	}
	if target.view == nil {
		t.Fatalf("the leaf scrolled to is not shaped")
	}
	if !strings.Contains(target.view.disp.text, "Paragraph 700") {
		t.Fatalf("the scrolled-to leaf shows %q", target.view.disp.text)
	}
	// The leaves just below it are on screen too, and the layout shapes a
	// screen above and below for the scroll that follows.
	below := h.e.leaves[at+1 : min(at+4, len(h.e.leaves))]
	for _, l := range below {
		if l.kind != kPara {
			continue
		}
		if l.view == nil {
			t.Fatalf("the leaf at y=%.0f just below the target is not shaped, though it is on screen", l.y)
		}
	}
	if l := h.e.leaves[at/2]; l.view != nil {
		t.Fatalf("leaf %d is shaped, though it is far above the viewport", at/2)
	}
	if n, total := shapedLeaves(h.e), len(h.e.leaves); n > total/4 {
		t.Fatalf("scrolling shaped %d of %d leaves", n, total)
	}
	// The scrolled-to leaf is inside the viewport, not merely shaped.
	if target.y+target.h < h.e.scroll.Y || target.y > h.e.scroll.Y+h.e.viewH {
		t.Fatalf("the target is at y=%.0f h=%.0f, outside the viewport [%.0f, %.0f]",
			target.y, target.h, h.e.scroll.Y, h.e.scroll.Y+h.e.viewH)
	}
}

// TestVirtualLayoutTyping checks that typing in a scrolled-to leaf updates
// its view: the leaves the document still holds views of have to be the
// ones the new source shows.
func TestVirtualLayoutTyping(t *testing.T) {
	h := newHarness(t, "")
	h.e.SetSource(proseSource(1200))
	h.frame()
	target := paragraphLeaf(t, h.e, 700)
	h.e.ScrollToOffset(target.start)
	for i := 0; i < 3; i++ {
		h.frame()
	}
	h.typ("x")

	if got := h.e.Source(); !strings.Contains(got, "xParagraph 700") {
		t.Fatal("the typed character was not inserted at the caret")
	}
	// The parse after the edit builds new leaves: the leaf the test held is
	// the one of the source before.
	l := h.e.leafAt(h.e.caret)
	if l == nil || l.view == nil {
		t.Fatal("the caret's leaf is not shaped after typing")
	}
	if got := l.view.disp.text; !strings.Contains(got, "xParagraph 700") {
		t.Fatalf("the caret's leaf shows %q, want the typed text", got)
	}
	if l.y+l.h < h.e.scroll.Y || l.y > h.e.scroll.Y+h.e.viewH {
		t.Fatalf("the caret's leaf is outside the viewport after typing")
	}
}

// TestVirtualLayoutReestimatesAfterResize checks that the leaves a
// narrower column leaves unshaped are measured again: their text wraps
// more, so the heights the old width gave them would put everything
// below them in the wrong place.
func TestVirtualLayoutReestimatesAfterResize(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 1200; i++ {
		fmt.Fprintf(&b, "Paragraph %d: %s\n\n", i,
			strings.Repeat("the quick brown fox jumps over the lazy dog. ", 8))
	}
	h := newHarness(t, "")
	h.e.SetSource(b.String())
	h.frame()

	var target *leaf
	for _, l := range h.e.leaves {
		if l.kind == kPara && l.view == nil {
			target = l
			break
		}
	}
	if target == nil {
		t.Fatal("every leaf is shaped; none is left to estimate")
	}
	before := target.h
	if before <= 0 {
		t.Fatalf("the unshaped leaf has height %.1f", before)
	}

	h.tt.SetSize(600, 700)
	h.frame()
	h.frame()

	if target.view != nil {
		t.Fatal("the leaf was shaped, though it is still below the viewport")
	}
	if target.h <= before {
		t.Fatalf("the leaf's height is %.1f after the column narrowed from its %.1f", target.h, before)
	}
}

// TestVirtualLayoutTable checks that a table the caret is scrolled to is
// laid out in a virtual document: its cells positioned and shaped, and
// the cell typed in shows the new text.
func TestVirtualLayoutTable(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 600; i++ {
		fmt.Fprintf(&b, "Paragraph %d: the quick brown fox jumps over the lazy dog.\n\n", i)
	}
	b.WriteString("| left | right |\n| --- | --- |\n| one | two |\n\n")
	for i := 600; i < 1200; i++ {
		fmt.Fprintf(&b, "Paragraph %d: the quick brown fox jumps over the lazy dog.\n\n", i)
	}

	h := newHarness(t, "")
	h.e.SetSource(b.String())
	h.frame()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
		}
	}
	if table == nil {
		t.Fatal("no table in the document")
	}
	cell := table.tab.rows[1][0]
	if cell.view != nil {
		t.Fatal("the table is laid out before it is scrolled to")
	}

	h.e.ScrollToOffset(table.start)
	for i := 0; i < 4; i++ {
		h.frame()
	}
	if table.h <= 0 || table.geom == nil {
		t.Fatal("the table the caret is scrolled to is not laid out")
	}
	if cell.view == nil {
		t.Fatal("a cell of the table on screen is not shaped")
	}
	if got := cell.view.disp.text; !strings.Contains(got, "one") {
		t.Fatalf("the cell shows %q, want its text", got)
	}

	h.e.Select(cell.start, cell.start)
	h.frame()
	h.frame()
	h.typ("z")
	if got := h.e.Source(); !strings.Contains(got, "| zone |") {
		t.Fatalf("the typed character is not in the table: %q", got)
	}
	// The edit built new leaves and cells: look them up again.
	table = nil
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
		}
	}
	if table == nil {
		t.Fatal("no table after the edit")
	}
	cell = table.tab.rows[1][0]
	if got := cell.view.disp.text; !strings.Contains(got, "zone") {
		t.Fatalf("the cell shows %q after typing, want %q", got, "zone")
	}
}

// TestSmallDocumentIsLaidOutWhole checks that a document below the
// threshold keeps exact positions: every leaf is shaped, wherever the
// viewport is.
func TestSmallDocumentIsLaidOutWhole(t *testing.T) {
	h := newHarness(t, proseSource(20))
	// Twenty paragraphs, and the blank lines between them.
	if n := len(blocks(h.e)); n != 20 {
		t.Fatalf("%d blocks, want 20", n)
	}
	if n, want := len(h.e.leaves), 2*20+1; n != want {
		t.Fatalf("%d leaves, want %d", n, want)
	}
	if n, want := shapedLeaves(h.e), len(blocks(h.e)); n != want {
		t.Fatalf("%d of %d blocks shaped, want them all", n, want)
	}
}
