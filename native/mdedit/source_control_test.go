package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// ink reports whether anything is painted inside r.
func ink(h *harness, r ui.Rect) bool {
	img := h.tt.Image()
	bg := img.RGBAAt(900, 600)
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			if img.RGBAAt(x, y) != bg {
				return true
			}
		}
	}
	return false
}

// TestASourceControlShowsABlockAsMarkdown: a block draws itself, and a
// control at its top right corner (the `</>` of a block's tools) shows the
// Markdown it is written as: the table stops being cells and becomes the
// lines of its source, which is what the caret then edits.
func TestASourceControlShowsABlockAsMarkdown(t *testing.T) {
	const src = "| 甲 | 乙 |\n| --- | --- |\n| a | b |"
	h := newHarness(t, src)
	var tbl *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			tbl = l
		}
	}
	if tbl == nil {
		t.Fatal("the source made no table")
	}
	chip := h.e.sourceChip(tbl)
	if ink(h, chip) {
		t.Fatal("the control is painted before the pointer is on the block")
	}
	// The pointer on the block shows its tools.
	h.tt.Move(h.e.colX+40, tbl.y+20)
	h.frame()
	if !ink(h, chip) {
		t.Fatal("the control is not painted while the pointer is on the block")
	}
	// The control turns the block into its source.
	h.tt.ClickAt(chip.X+6, chip.Y+4)
	h.frame()
	l := h.e.leafAt(h.e.caret)
	if l == nil || l.kind != kRaw || l.view == nil {
		t.Fatalf("the block is not its source: %+v", l)
	}
	if !strings.Contains(l.view.disp.text, "| --- |") {
		t.Fatalf("the source shows %q", l.view.disp.text)
	}
	for _, u := range h.e.units() {
		if u.table != nil {
			t.Fatal("the table's cells are still units while it shows its source")
		}
	}
	// And back again.
	chip = h.e.sourceChip(l)
	h.tt.ClickAt(chip.X+6, chip.Y+4)
	h.frame()
	back := false
	for _, x := range h.e.leaves {
		if x.kind == kTable && x.tab != nil {
			back = true
		}
	}
	if !back {
		t.Fatal("the control did not draw the table again")
	}
}

// TestTheControlGoesWithThePointer: the tools belong to the block the
// pointer is on, and leave with it.
func TestTheControlGoesWithThePointer(t *testing.T) {
	h := newHarness(t, "前\n\n| 甲 | 乙 |\n| --- | --- |\n| a | b |\n\n后")
	var tbl *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			tbl = l
		}
	}
	h.tt.Move(h.e.colX+40, tbl.y+20)
	h.frame()
	if h.e.hoverBlock != tbl.lineStart {
		t.Fatalf("hoverBlock = %d, want the table at %d", h.e.hoverBlock, tbl.lineStart)
	}
	chip := h.e.sourceChip(tbl)
	h.tt.Move(h.e.colX+40, tbl.y+tbl.h+80)
	h.frame()
	if h.e.hoverBlock != -1 {
		t.Fatalf("hoverBlock = %d, want none", h.e.hoverBlock)
	}
	if ink(h, chip) {
		t.Fatal("the control stayed after the pointer left the block")
	}
}
