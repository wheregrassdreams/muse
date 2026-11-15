package mdedit

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestHorizontalScrollCoreKeepsIndependentBlockState(t *testing.T) {
	code := "```text\n" + strings.Repeat("0123456789abcdef", 60) + "\n```"
	tex := "\\[\n" + strings.Repeat("x+", 100) + "x\n\\]"
	table := "| A | B | C |\n| --- | --- | --- |\n| " + strings.Repeat("wide", 80) + " | middle | end |"
	src := code + "\n\n" + tex + "\n\n" + table + "\n\nend"
	h := newHarness(t, src)
	e := h.e
	e.layoutIfNeeded()

	var codeLeaf, mathLeaf, tableLeaf *leaf
	for _, l := range e.leaves {
		switch l.kind {
		case kCode:
			codeLeaf = l
		case kMath:
			mathLeaf = l
		case kTable:
			tableLeaf = l
		}
	}
	if codeLeaf == nil || mathLeaf == nil || tableLeaf == nil {
		t.Fatal("fixture did not produce code, math, and table blocks")
	}
	for name, l := range map[string]*leaf{"code": codeLeaf, "math": mathLeaf, "table": tableLeaf} {
		if w, vw := blockContentWidth(l), blockViewport(l).W; w <= vw {
			t.Fatalf("%s fixture does not overflow: content=%v viewport=%v", name, w, vw)
		}
		if !e.setBlockHorizontal(l, 24) || l.horizontal.x != 24 {
			t.Fatalf("%s block did not retain an independent horizontal offset", name)
		}
	}
	e.setBlockHorizontal(mathLeaf, 0)
	e.setBlockHorizontal(tableLeaf, 0)

	codeOffset := codeLeaf.codeLines()[0].off + 40
	codeCaret, ok := e.caretRect(codeOffset)
	if !ok {
		t.Fatal("long code caret has no geometry")
	}
	beforeScroll, beforeSource := e.scroll.Y, e.Source()
	if !e.setBlockHorizontal(codeLeaf, 120) {
		t.Fatal("long code did not accept a horizontal offset")
	}
	e.revealHorizontalCaret(codeOffset)
	if codeLeaf.horizontal.x == 0 {
		t.Fatal("caret reveal reset the horizontal offset")
	}
	if got := e.hitTest(codeCaret.X-codeLeaf.horizontal.x+120, codeCaret.Y+codeCaret.H/2); got == codeOffset {
		t.Fatal("hit test unexpectedly used pre-scroll code coordinates")
	}
	codeCaret, _ = e.caretRect(codeOffset)
	if got := e.hitTest(codeCaret.X+0.1, codeCaret.Y+codeCaret.H/2); got != codeOffset {
		t.Fatalf("code caret/hit round trip = %d, want %d", got, codeOffset)
	}

	cell := tableLeaf.tab.rows[1][0]
	cellOffset := cell.start + 12
	e.setBlockHorizontal(tableLeaf, 80)
	cellCaret, ok := e.caretRect(cellOffset)
	if !ok {
		t.Fatal("scrolled table cell caret has no geometry")
	}
	if got := e.hitTest(cellCaret.X+0.1, cellCaret.Y+cellCaret.H/2); got != cellOffset {
		t.Fatalf("table cell caret/hit round trip = %d, want %d", got, cellOffset)
	}
	if e.scroll.Y != beforeScroll || e.Source() != beforeSource {
		t.Fatalf("horizontal movement changed source or vertical scroll: scroll=%v sourceChanged=%v", e.scroll.Y, e.Source() != beforeSource)
	}
	e.scroll.MaxY, e.scroll.Y = 500, 30
	beforeX := codeLeaf.horizontal.x
	e.scrollBlock(ui.InputEvent{Kind: ui.InputScroll, X: codeLeaf.x + 10, Y: codeLeaf.y + 10, DX: 20})
	if e.scroll.Y != 30 || codeLeaf.horizontal.x == beforeX {
		t.Fatalf("horizontal-only input changed vertical state or failed horizontally: y=%v x=%v", e.scroll.Y, codeLeaf.horizontal.x)
	}
	e.scrollBlock(ui.InputEvent{Kind: ui.InputScroll, X: codeLeaf.x + 10, Y: codeLeaf.y + 10, DX: 10, DY: 8})
	if e.scroll.Y != 38 {
		t.Fatalf("mixed-axis input swallowed vertical movement: y=%v, want 38", e.scroll.Y)
	}

	// Editing the existing table keeps its presentation offset, clamped to
	// the new range if its geometry changes.
	e.setBlockHorizontal(tableLeaf, 80)
	e.replace(cell.start, cell.start+1, "W")
	e.layoutIfNeeded()
	var editedTable *leaf
	for _, l := range e.leaves {
		if l.kind == kTable {
			editedTable = l
		}
	}
	if editedTable == nil {
		t.Fatal("editing an existing cell removed its table")
	}
	if editedTable.horizontal.x != 80 {
		t.Fatalf("editing the existing table discarded its horizontal viewport: got=%v want=80", editedTable.horizontal.x)
	}

	// Reopening/replacing the whole source resets presentation state, even
	// when the replacement has identical source text and block positions.
	e.setBlockHorizontal(codeLeaf, 100)
	e.SetSource(src)
	e.layoutIfNeeded()
	for _, l := range e.leaves {
		if l.horizontal.x != 0 {
			t.Fatalf("source replacement carried a stale horizontal offset on %v: %v", l.kind, l.horizontal.x)
		}
	}
}

func TestHorizontalScrollIndicatorTracksActualMovementOnly(t *testing.T) {
	src := "```text\n" + strings.Repeat("wide-line-", 80) + "\n```\n\nend"
	h := newHarness(t, src)
	e := h.e
	e.layoutIfNeeded()
	l := e.leaves[0]
	if blockContentWidth(l) <= blockViewport(l).W {
		t.Fatal("fixture must overflow horizontally")
	}
	track, _ := blockHorizontalBarRects(blockViewport(l), blockContentWidth(l), l.horizontal.x)

	// Painting and hovering alone do not create a scroll indicator.
	h.tt.Move(track.X+10, track.Y+1)
	h.frame()
	if l.horizontal.shown {
		t.Fatal("hover showed the horizontal bar")
	}

	if !e.scrollBlock(ui.InputEvent{Kind: ui.InputScroll, X: l.x + 10, Y: l.y + 10, DX: 40}) {
		t.Fatal("horizontal wheel was not routed to the overflowing block")
	}
	h.frame()
	if !l.horizontal.shown {
		t.Fatal("actual horizontal movement did not show the bar")
	}

	h.tt.Advance(701 * time.Millisecond)
	h.frame()
	if l.horizontal.shown {
		t.Fatal("expired horizontal bar remained visible")
	}

	maxX := blockContentWidth(l) - blockViewport(l).W
	e.setBlockHorizontal(l, maxX)
	h.frame()
	if !l.horizontal.shown {
		t.Fatal("actual movement to the edge did not show the bar")
	}
	beforeEdge := l.horizontal.x
	e.scrollBlock(ui.InputEvent{Kind: ui.InputScroll, X: l.x + 10, Y: l.y + 10, DX: 100})
	if l.horizontal.x != beforeEdge {
		t.Fatalf("edge scroll changed offset from %v to %v", beforeEdge, l.horizontal.x)
	}
	h.frame()
	if !l.horizontal.shown {
		t.Fatal("bar disappeared before its actual-movement lifetime expired")
	}
	h.tt.Advance(701 * time.Millisecond)
	h.frame()
	if l.horizontal.shown {
		t.Fatal("edge interaction kept an expired bar visible")
	}
	e.scrollBlock(ui.InputEvent{Kind: ui.InputScroll, X: l.x + 10, Y: l.y + 10, DX: 100})
	h.frame()
	if l.horizontal.shown {
		t.Fatal("ineffective scrolling at the edge revived an expired bar")
	}
}

func TestHorizontalCodeDocumentRetainsViewportWhileEditing(t *testing.T) {
	src := strings.Repeat("wide_code_", 80)
	e := New(src, Options{Code: "go", FontSize: 16, MaxWidth: 420})
	h := &harness{t: t, e: e}
	h.tt = ui.NewTester(func(c *ui.Context) { e.View(c).Fill() }, 620, 400)
	h.frame()
	h.frame()
	e.setBlockHorizontal(e.leaves[0], 120)
	h.frame()
	e.replace(25, 26, "W")
	e.commit(25, 25)
	e.layoutIfNeeded()
	if got := e.leaves[0].horizontal.x; got != 120 {
		t.Fatalf("editing a code document reset its horizontal viewport: got=%v want=120", got)
	}
	e.SetSource(src)
	e.layoutIfNeeded()
	if got := e.leaves[0].horizontal.x; got != 0 {
		t.Fatalf("opening a code document retained the previous document's viewport: %v", got)
	}
}
