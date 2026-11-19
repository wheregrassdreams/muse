package editor

import (
	helix "github.com/wheregrassdreams/muse-editor/text/helix"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPointerTableDragUpgradesAndLocksCellRegion(t *testing.T) {
	h := newHarness(t, "| A | B | C |\n| --- | --- | --- |\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	cellPoint := func(row, col int) (float32, float32) {
		return table.x + (table.geom.colX[col]+table.geom.colX[col+1])/2,
			table.y + (table.geom.rowY[row]+table.geom.rowY[row+1])/2
	}
	x, y := cellPoint(1, 0)
	h.e.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, Clicks: 1, X: x, Y: y})
	h.e.input(ui.InputEvent{Kind: ui.InputPointerMove, Button: 0, X: x + 1, Y: y})
	if h.e.tableSelection != nil {
		t.Fatal("same-cell drag unexpectedly became a structural selection")
	}
	x, y = cellPoint(2, 2)
	h.e.input(ui.InputEvent{Kind: ui.InputPointerMove, Button: 0, X: x, Y: y})
	s := h.e.tableSelection
	if s == nil || !s.cellRegion || s.anchorRow != 1 || s.anchorCol != 0 || s.focusRow != 2 || s.focusCol != 2 {
		t.Fatalf("cross-cell drag did not create the anchored rectangle: %+v", s)
	}
	x, y = cellPoint(1, 0)
	h.e.input(ui.InputEvent{Kind: ui.InputPointerMove, Button: 0, X: x, Y: y})
	if s := h.e.tableSelection; s == nil || !s.cellRegion || s.anchorRow != 1 || s.anchorCol != 0 || s.focusRow != 1 || s.focusCol != 0 {
		t.Fatalf("returning to anchor lost region mode: %+v", s)
	}
	h.e.input(ui.InputEvent{Kind: ui.InputPointerUp, Button: 0, X: x, Y: y})
	if h.e.tableCellDrag != nil {
		t.Fatal("pointer-up retained transient table drag state")
	}
}

func TestTableVisualRegionVimMotionAndWholeRow(t *testing.T) {
	h := newHarness(t, "| A | B | C |\n| --- | --- | --- |\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.e.setSessionHead(table.tab.rows[1][1].start)
	h.e.vimController().SetMode(helix.ModeNormal)
	h.e.modalKey(ui.KeyV, ui.Ctrl)
	if s := h.e.tableSelection; s == nil || s.axis != tableCells || s.anchorRow != 1 || s.anchorCol != 1 {
		t.Fatalf("Ctrl+V did not begin at current cell: %+v", s)
	}
	sendVim(h, "lj")
	if s := h.e.tableSelection; s == nil || s.focusRow != 2 || s.focusCol != 2 {
		t.Fatalf("h/j/l motion did not expand cell rectangle: %+v", s)
	}
	h.e.vimController().SetMode(helix.ModeNormal)
	h.e.tableSelection = nil
	h.e.setSessionHead(table.tab.rows[1][1].start)
	sendVim(h, "V")
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 1 || s.last != 1 {
		t.Fatalf("V did not select full row: %+v", s)
	}
}

func TestTableVisualWholeRowExtendsAndShrinks(t *testing.T) {
	h := newHarness(t, "| A | B | C |\n| --- | --- | --- |\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |\n| 7 | 8 | 9 |")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.e.setSessionHead(table.tab.rows[1][1].start)
	sendVim(h, "Vj")
	s := h.e.tableSelection
	if s == nil || s.axis != tableRows || s.first != 1 || s.last != 2 {
		t.Fatalf("Vj did not extend a full-row region: %+v", s)
	}
	sendVim(h, "k")
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 1 || s.last != 1 {
		t.Fatalf("Vk did not shrink to the anchored row: %+v", s)
	}
}

func TestTableCellRegionVimMotionFourDirections(t *testing.T) {
	for _, tc := range []struct {
		key      string
		row, col int
	}{
		{key: "h", row: 1, col: 0}, {key: "l", row: 1, col: 2},
		{key: "k", row: 0, col: 1}, {key: "j", row: 2, col: 1},
	} {
		t.Run(tc.key, func(t *testing.T) {
			h := newHarness(t, "| A | B | C |\n| --- | --- | --- |\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |\n| 7 | 8 | 9 |")
			h.e.SetKeymap("vim")
			h.e.layoutIfNeeded()
			table := documentTestTable(t, h.e)
			h.e.setSessionHead(table.tab.rows[1][1].start)
			h.e.modalKey(ui.KeyV, ui.Ctrl)
			sendVim(h, tc.key)
			s := h.e.tableSelection
			if s == nil || s.anchorRow != 1 || s.anchorCol != 1 || s.focusRow != tc.row || s.focusCol != tc.col {
				t.Fatalf("motion %s changed wrong region: %+v", tc.key, s)
			}
		})
	}
}

func TestTableCellRegionVimMotionKeepsAnchorAcrossSteps(t *testing.T) {
	h := newHarness(t, "| A | B | C |\n| --- | --- | --- |\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |\n| 7 | 8 | 9 |")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.e.setSessionHead(table.tab.rows[1][1].start)
	h.e.modalKey(ui.KeyV, ui.Ctrl)
	for _, step := range []struct {
		key      string
		row, col int
	}{{"h", 1, 0}, {"k", 0, 0}, {"l", 0, 1}, {"j", 1, 1}} {
		sendVim(h, step.key)
		s := h.e.tableSelection
		if s == nil {
			t.Fatalf("after %s region disappeared", step.key)
		}
		if s.anchorRow != 1 || s.anchorCol != 1 || s.focusRow != step.row || s.focusCol != step.col {
			t.Fatalf("after %s anchor/focus=(%d,%d)/(%d,%d), want (1,1)/(%d,%d)", step.key, s.anchorRow, s.anchorCol, s.focusRow, s.focusCol, step.row, step.col)
		}
	}
}

func TestCellRegionKeepsFocusMotionAcrossFullRowShape(t *testing.T) {
	h := newHarness(t, "| A | B | C |\n| --- | --- | --- |\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |\n| 7 | 8 | 9 |")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.e.setSessionHead(table.tab.rows[1][0].start)
	h.e.modalKey(ui.KeyV, ui.Ctrl)
	steps := []struct {
		key      string
		row, col int
		axis     tableAxis
	}{{"l", 1, 1, tableCells}, {"l", 1, 2, tableRows}, {"j", 2, 2, tableRows}, {"k", 1, 2, tableRows}, {"h", 1, 1, tableCells}, {"k", 0, 1, tableCells}}
	for _, step := range steps {
		sendVim(h, step.key)
		s := h.e.tableSelection
		if s == nil || !s.cellRegion || s.anchorRow != 1 || s.anchorCol != 0 || s.focusRow != step.row || s.focusCol != step.col || s.axis != step.axis {
			t.Fatalf("after %s got %+v, want focus=(%d,%d) axis=%d with fixed anchor (1,0)", step.key, s, step.row, step.col, step.axis)
		}
	}
}

func TestVimVisualTextSelectionPromotesAcrossCellsAndStaysStructural(t *testing.T) {
	h := newHarness(t, "| A | B |\n| --- | --- |\n| alpha | beta |")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.e.setSessionHead(table.tab.rows[1][0].start)
	sendVim(h, "v")
	sendVim(h, "l")
	if s := h.e.tableSelection; s != nil && s.cellRegion {
		t.Fatalf("same-cell text motion prematurely promoted: %+v", s)
	}
	anchor := h.e.Session.Selection().Anchor
	for n := 0; n < 30 && (h.e.tableSelection == nil || !h.e.tableSelection.cellRegion); n++ {
		sendVim(h, "l")
	}
	s := h.e.tableSelection
	if s == nil || !s.cellRegion || s.anchorRow != 1 || s.anchorCol != 0 || s.focusRow != 1 || s.focusCol != 1 {
		t.Fatalf("cross-cell v motion did not promote to rectangle: %+v", s)
	}
	if h.e.Session.Selection().Anchor != anchor {
		t.Fatalf("promotion changed the text anchor offset: %d -> %d", anchor, h.e.Session.Selection().Anchor)
	}
	sendVim(h, "h")
	if s := h.e.tableSelection; s == nil || !s.cellRegion || s.anchorCol != 0 || s.focusCol != 0 {
		t.Fatalf("moving back to the origin downgraded Visual selection: %+v", s)
	}
}

func TestCellRegionMotionPastTableEdgesReturnsToDocumentSelection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		row, col int
		key      string
	}{{"left", 1, 0, "h"}, {"right", 1, 1, "l"}, {"up", 0, 0, "k"}, {"down", 1, 0, "j"}} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, "before\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\nafter")
			h.e.SetKeymap("vim")
			h.e.layoutIfNeeded()
			table := documentTestTable(t, h.e)
			h.e.setSessionHead(table.tab.rows[tc.row][tc.col].start)
			h.e.modalKey(ui.KeyV, ui.Ctrl)
			sendVim(h, tc.key)
			if s := h.e.tableSelection; s != nil && s.cellRegion {
				t.Fatalf("motion past %s table edge froze structural region: %+v", tc.name, s)
			}
			if h.e.Session.Selection().Anchor == h.e.Caret() {
				t.Fatal("leaving table collapsed the document selection")
			}
		})
	}
}

func TestPointerTableRegionLeavesTableForDocumentSelection(t *testing.T) {
	h := newHarness(t, "| A | B |\n| --- | --- |\n| 1 | 2 |\n\nparagraph\n\n| C | D |\n| --- | --- |\n| 3 | 4 |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	startX := table.x + (table.geom.colX[0]+table.geom.colX[1])/2
	startY := table.y + (table.geom.rowY[1]+table.geom.rowY[2])/2
	h.e.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, Clicks: 1, X: startX, Y: startY})
	other := h.e.leaves[len(h.e.leaves)-1]
	x, y := other.x+other.w/2, other.y+other.h/2
	h.e.input(ui.InputEvent{Kind: ui.InputPointerMove, Button: 0, X: x, Y: y})
	if h.e.tableSelection != nil {
		t.Fatalf("drag leaving table retained structural selection: %+v", h.e.tableSelection)
	}
	if h.e.Session.Selection().Anchor == h.e.Caret() {
		t.Fatal("drag leaving table did not continue document text selection")
	}
}
