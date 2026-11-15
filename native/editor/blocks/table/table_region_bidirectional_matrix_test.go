package mdedit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

const bidirectionalTableSource = "| Local file | Source page | Creator / source | License |\n| --- | --- | --- | --- |\n| `Attachments/landscape.webp` | [File:Webp (1).webp](https://commons.wikimedia.org/wiki/File:Webp_(1).webp) | Churchill Machine Tool Co. Ltd.; source: picryl.com | [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/) |\n| `Attachments/diagram.png` | [File:A landscape - PNG](https://example.test/a) | Public-domain painting reproduction; National Gallery of Ireland | Public domain artwork |\n| another file | [File:B](https://example.test/b) | Another creator | CC BY |"

func physicalVimStep(t *testing.T, h *harness, key ui.Key, text string) {
	t.Helper()
	h.tt.Key(0, key)
	h.frame()
	h.tt.Type(text)
	h.frame()
}

func clickTableCellAtPoint(t *testing.T, h *harness, row, col int, fx, fy float32) {
	t.Helper()
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	x := table.x + table.geom.colX[col] + (table.geom.colX[col+1]-table.geom.colX[col])*fx
	y := table.y + table.geom.rowY[row] + (table.geom.rowY[row+1]-table.geom.rowY[row])*fy - h.e.scroll.Y
	if gotRow, gotCol, inside := tableCellIndexAt(table, x, y+h.e.scroll.Y); !inside || gotRow != row || gotCol != col {
		t.Fatalf("test pointer maps to (%d,%d), inside=%v; want (%d,%d)", gotRow, gotCol, inside, row, col)
	}
	h.tt.Move(x, y)
	h.tt.Press(x, y)
	h.tt.Release(x, y)
	h.frame()
	_, gotRow, gotCol := cellAt(table, h.e.caret)
	if gotRow != row || gotCol != col {
		t.Fatalf("pointer placed caret in (%d,%d), want (%d,%d), offset=%d", gotRow, gotCol, row, col, h.e.caret)
	}
}

func assertCellRegionFocus(t *testing.T, h *harness, row, col int) {
	t.Helper()
	s := h.e.tableSelection
	if s == nil || !s.cellRegion {
		t.Fatalf("expected active cell region at (%d,%d), got %+v", row, col, s)
	}
	table := documentTestTable(t, h.e)
	top, left, bottom, right := tableCellRegion{lineStart: s.lineStart, anchorRow: s.anchorRow, anchorCol: s.anchorCol, focusRow: s.focusRow, focusCol: s.focusCol}.bounds()
	if s.focusRow != row || s.focusCol != col || row < top || row > bottom || col < left || col > right {
		t.Fatalf("focus escaped its selection: want (%d,%d), selection=%+v bounds=[%d,%d]x[%d,%d]", row, col, s, top, bottom, left, right)
	}
	caret, ok := h.e.tableSelectionCaretRect()
	if !ok {
		t.Fatalf("focus (%d,%d) has no structural caret geometry", row, col)
	}
	wantX := tableContentX(table) + table.geom.colX[col]
	wantY := table.y + table.geom.rowY[row]
	dx, dy := caret.X-wantX, caret.Y-wantY
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx > 1 || dy > 1 {
		t.Fatalf("caret geometry %+v disagrees with focus (%d,%d): want x=%f y=%f", caret, row, col, wantX, wantY)
	}
	if h.e.tableBridge != nil || h.e.modal.line || h.e.selectionLinewise || h.e.selectionInclusive {
		t.Fatalf("cell region is mixed with text/bridge selection: selection=%+v bridge=%+v mode=%s line=%v linewise=%v inclusive=%v", s, h.e.tableBridge, h.e.modal.mode, h.e.modal.line, h.e.selectionLinewise, h.e.selectionInclusive)
	}
}

func TestLowercaseVLongTableHorizontalRowWrapAndBidirectionalFocus(t *testing.T) {
	h := &harness{t: t, e: New(bidirectionalTableSource, Options{FontSize: 18, MaxWidth: 820, PadX: 32})}
	h.e.SetKeymap("vim")
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { h.e.View(c).Grow(1) })
	}, 1040, 640)
	h.frame()
	h.e.Focus()
	h.frame()
	table := documentTestTable(t, h.e)
	// Click below the rendered license link in the tall first body row. This is
	// the screenshot's empty-cell-area path rather than a source-offset setup.
	clickTableCellAtPoint(t, h, 1, 3, 0.55, 0.78)
	physicalVimStep(t, h, ui.KeyV, "v")

	// Cross from the License cell through the other cells and reach row 1 col 0.
	for i := 0; i < 100; i++ {
		physicalVimStep(t, h, ui.KeyH, "h")
		if s := h.e.tableSelection; s != nil && s.cellRegion && s.focusRow == 1 && s.focusCol == 0 {
			break
		}
		if i == 99 {
			t.Fatalf("h did not reach first body cell: selection=%+v caret=%d", h.e.tableSelection, h.e.caret)
		}
	}
	assertCellRegionFocus(t, h, 1, 0)
	bodyEdge := h.e.tableNavigationState()

	// One more h wraps to the header at the same column. The selected bounds
	// must include the entire header/body rectangle and the caret must follow it.
	physicalVimStep(t, h, ui.KeyH, "h")
	assertCellRegionFocus(t, h, 0, 0)
	if s := h.e.tableSelection; s.first != 0 || s.last != 1 || s.anchorRow != 1 || s.anchorCol != 3 {
		t.Fatalf("header wrap lost the fixed body anchor or expanded wrong bounds: %+v", s)
	}

	// An orthogonal move and its inverse must not discard the pending wrap
	// transition: h-wrap -> j -> k -> l returns to the exact pre-wrap snapshot.
	headerWrap := h.e.tableNavigationState()
	physicalVimStep(t, h, ui.KeyJ, "j")
	assertCellRegionFocus(t, h, 1, 0)
	physicalVimStep(t, h, ui.KeyK, "k")
	if got := h.e.tableNavigationState(); !sameTableNavigationState(got, headerWrap) {
		t.Fatalf("k failed to reverse j after h-wrap\nwant: %+v\n got: %+v", headerWrap, got)
	}
	physicalVimStep(t, h, ui.KeyL, "l")
	if got := h.e.tableNavigationState(); !sameTableNavigationState(got, bodyEdge) {
		t.Fatalf("l failed to reverse the preceding h transition\nwant: %+v\n got: %+v", bodyEdge, got)
	}
	assertCellRegionFocus(t, h, 1, 0)

	// Vertical movement from the body edge must expand/shrink the same rectangle,
	// not leave a stale header/body highlight behind a row-2 caret.
	physicalVimStep(t, h, ui.KeyK, "k")
	assertCellRegionFocus(t, h, 0, 0)
	physicalVimStep(t, h, ui.KeyJ, "j")
	assertCellRegionFocus(t, h, 1, 0)
	physicalVimStep(t, h, ui.KeyJ, "j")
	assertCellRegionFocus(t, h, 2, 0)
	if s := h.e.tableSelection; s.first != 1 || s.last != 2 {
		t.Fatalf("second body row focus has stale header bounds: %+v", s)
	}
	physicalVimStep(t, h, ui.KeyK, "k")
	assertCellRegionFocus(t, h, 1, 0)

	// Continue horizontally, then reverse and replay the exact inverse path.
	physicalVimStep(t, h, ui.KeyL, "l")
	assertCellRegionFocus(t, h, 1, 1)
	physicalVimStep(t, h, ui.KeyL, "l")
	assertCellRegionFocus(t, h, 1, 2)
	physicalVimStep(t, h, ui.KeyH, "h")
	assertCellRegionFocus(t, h, 1, 1)
	physicalVimStep(t, h, ui.KeyH, "h")
	assertCellRegionFocus(t, h, 1, 0)

	if table == nil {
		t.Fatal("table projection disappeared during navigation")
	}
}

func TestLowercaseVHorizontalWrapAcrossDifferentStartCellsAndDirections(t *testing.T) {
	for _, tc := range []struct {
		name      string
		row, col  int
		firstMove ui.Key
		text      string
		lastCol   int
	}{
		{name: "license-left-to-header", row: 1, col: 3, firstMove: ui.KeyH, text: "h", lastCol: 0},
		{name: "first-cell-right-to-next-row", row: 1, col: 0, firstMove: ui.KeyL, text: "l", lastCol: 3},
		{name: "middle-cell-left", row: 2, col: 2, firstMove: ui.KeyH, text: "h", lastCol: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, bidirectionalTableSource)
			h.e.SetKeymap("vim")
			table := documentTestTable(t, h.e)
			h.caretAt(table.tab.rows[tc.row][tc.col].start)
			h.e.modalText("v")
			for i := 0; i < 80; i++ {
				h.e.modalText(tc.text)
				s := h.e.tableSelection
				if s != nil && s.cellRegion && s.focusRow != tc.row {
					assertCellRegionFocus(t, h, s.focusRow, s.focusCol)
					return
				}
				if s != nil && s.cellRegion && s.focusCol == tc.lastCol {
					before := h.e.tableNavigationState()
					h.e.modalText(tc.text)
					s = h.e.tableSelection
					if s == nil || !s.cellRegion || s.focusRow == tc.row {
						t.Fatalf("horizontal boundary motion did not cross to adjacent row: %+v", s)
					}
					assertCellRegionFocus(t, h, s.focusRow, s.focusCol)
					opposite := "h"
					if tc.text == "h" {
						opposite = "l"
					}
					h.e.modalText(opposite)
					if !sameTableNavigationState(h.e.tableNavigationState(), before) {
						t.Fatalf("opposite horizontal motion did not restore the prior cell focus")
					}
					return
				}
			}
			t.Fatalf("did not reach horizontal wrap from (%d,%d): selection=%+v", tc.row, tc.col, h.e.tableSelection)
		})
	}
}

func TestLowercaseVHeaderRegionExitThenReverseIntoLaterRowTracksCaret(t *testing.T) {
	src := "Before paragraph.\n\n" + bidirectionalTableSource + "\n\nAfter paragraph."
	h := &harness{t: t, e: New(src, Options{FontSize: 18, MaxWidth: 820, PadX: 32})}
	h.e.SetKeymap("vim")
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { h.e.View(c).Grow(1) })
	}, 1040, 680)
	h.frame()
	h.e.Focus()
	h.frame()
	documentTestTable(t, h.e)
	clickTableCellAtPoint(t, h, 1, 3, 0.55, 0.78)
	physicalVimStep(t, h, ui.KeyV, "v")
	for i := 0; i < 100; i++ {
		physicalVimStep(t, h, ui.KeyH, "h")
		if s := h.e.tableSelection; s != nil && s.cellRegion && s.focusRow == 1 && s.focusCol == 0 {
			break
		}
		if i == 99 {
			t.Fatalf("could not reach first body cell: %+v", h.e.tableSelection)
		}
	}
	physicalVimStep(t, h, ui.KeyH, "h") // Header row, first column.
	assertCellRegionFocus(t, h, 0, 0)
	if s := h.e.tableSelection; s.first != 0 || s.last != 1 {
		t.Fatalf("header/body region was not selected before leaving: %+v", s)
	}
	physicalVimStep(t, h, ui.KeyH, "h") // Leave table to preceding document text.
	if h.e.tableSelection != nil || h.e.tableDocumentRegion == nil || h.e.tableDocumentRegion.first != 0 || h.e.tableDocumentRegion.last != 1 {
		t.Fatalf("leaving table did not preserve rows 0-1: active=%+v document=%+v", h.e.tableSelection, h.e.tableDocumentRegion)
	}
	physicalVimStep(t, h, ui.KeyL, "l") // Exact inverse of the table exit.
	assertCellRegionFocus(t, h, 0, 0)
	physicalVimStep(t, h, ui.KeyJ, "j")
	assertCellRegionFocus(t, h, 1, 0)
	physicalVimStep(t, h, ui.KeyJ, "j")
	assertCellRegionFocus(t, h, 2, 0)
	if s := h.e.tableSelection; s.first != 1 || s.last != 2 {
		t.Fatalf("row-2 caret retained stale header/body bounds: %+v", s)
	}
}

func TestLowercaseVReenteredTableReprojectsSuspendedRegionToCaret(t *testing.T) {
	src := "Before paragraph.\n\n" + bidirectionalTableSource + "\n\nAfter paragraph."
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[1][3].start)
	h.e.modalText("v")
	for i := 0; i < 100; i++ {
		h.e.modalText("h")
		if s := h.e.tableSelection; s != nil && s.cellRegion && s.focusRow == 1 && s.focusCol == 0 {
			break
		}
		if i == 99 {
			t.Fatalf("could not reach first body cell: %+v", h.e.tableSelection)
		}
	}
	h.e.modalText("h") // Include the header row.
	h.e.modalText("h") // Exit to text before the table.
	if h.e.tableSelection != nil || h.e.tableDocumentRegion == nil {
		t.Fatalf("expected suspended table region after exit: active=%+v document=%+v", h.e.tableSelection, h.e.tableDocumentRegion)
	}
	anchorRow, anchorCol := h.e.tableDocumentRegion.anchorRow, h.e.tableDocumentRegion.anchorCol
	if h.e.anchor < table.lineStart || h.e.anchor > blockEnd(h.e.src, table) {
		t.Fatalf("table anchor was not preserved in mixed selection: anchor=%d table=[%d,%d)", h.e.anchor, table.lineStart, blockEnd(h.e.src, table))
	}
	var external, suspendedStructure bool
	for _, part := range h.e.selectionParts() {
		external = external || part.structure == nil && part.source.Start < table.lineStart
		suspendedStructure = suspendedStructure || part.structure != nil && part.structure.focusRow == 0 && part.structure.focusCol == 0
	}
	if !external || !suspendedStructure {
		t.Fatalf("suspended selection lost external text or table region: parts=%+v", h.e.selectionParts())
	}

	// Move through the outside paragraph and back into the table with physical
	// Visual j input. Once the caret re-enters, resume the suspended rectangle
	// from its original anchor and continue vertically by whole cells.
	for i := 0; i < 8; i++ {
		physicalVimStep(t, h, ui.KeyJ, "j")
		_, caretRow, caretCol := cellAt(table, h.e.caret)
		if h.e.tableSelection == nil {
			if caretRow >= 0 {
				t.Fatalf("caret entered table cell (%d,%d) without resuming a cell region: document=%+v", caretRow, caretCol, h.e.tableDocumentRegion)
			}
			if h.e.tableDocumentRegion == nil {
				t.Fatalf("outside movement discarded suspended table region at step %d", i+1)
			}
			continue
		}
		if h.e.tableSelection.anchorRow != anchorRow || h.e.tableSelection.anchorCol != anchorCol {
			t.Fatalf("reentry changed the original table anchor: got (%d,%d), want (%d,%d)", h.e.tableSelection.anchorRow, h.e.tableSelection.anchorCol, anchorRow, anchorCol)
		}
		if h.e.tableDocumentRegion != nil {
			t.Fatalf("reentered table retained a second frozen region: %+v", h.e.tableDocumentRegion)
		}
		assertCellRegionFocus(t, h, h.e.tableSelection.focusRow, h.e.tableSelection.focusCol)
		if h.e.tableSelection.focusRow == 2 {
			if s := h.e.tableSelection; s.first != min(anchorRow, 2) || s.last != max(anchorRow, 2) {
				t.Fatalf("reentered region retained stale header/body bounds: %+v", s)
			}
			return
		}
	}
	t.Fatalf("Visual j did not reenter the table and reach row 2: caret=%d active=%+v document=%+v", h.e.caret, h.e.tableSelection, h.e.tableDocumentRegion)
}

func TestMixedSelectionReenteredCellRegionThemeShots(t *testing.T) {
	src := "Before paragraph.\n\n" + bidirectionalTableSource + "\n\nAfter paragraph."
	for _, dark := range []bool{false, true} {
		name, theme := "light", ui.LightTheme()
		if dark {
			name, theme = "dark", ui.DarkTheme()
		}
		t.Run(name, func(t *testing.T) {
			h := &harness{t: t, e: New(src, Options{FontSize: 18, MaxWidth: 820, PadX: 32})}
			h.e.SetKeymap("vim")
			h.tt = ui.NewTester(func(c *ui.Context) {
				ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() { h.e.View(c).Grow(1) })
			}, 1040, 680)
			h.frame()
			h.e.Focus()
			h.frame()
			table := documentTestTable(t, h.e)
			clickTableCellAtPoint(t, h, 1, 3, 0.55, 0.78)
			physicalVimStep(t, h, ui.KeyV, "v")
			for i := 0; i < 100; i++ {
				physicalVimStep(t, h, ui.KeyH, "h")
				if s := h.e.tableSelection; s != nil && s.cellRegion && s.focusRow == 1 && s.focusCol == 0 {
					break
				}
				if i == 99 {
					t.Fatalf("could not reach first body cell: %+v", h.e.tableSelection)
				}
			}
			physicalVimStep(t, h, ui.KeyH, "h")
			physicalVimStep(t, h, ui.KeyH, "h") // Suspend the region outside the table.
			focus := table.tab.rows[2][0]
			h.e.setCaret(focus.start+2, true)
			assertCellRegionFocus(t, h, 2, 0)
			if s := h.e.tableSelection; s.first != 1 || s.last != 2 {
				t.Fatalf("painted region bounds disagree with reentered focus: %+v", s)
			}
			h.shot("reentered-table-region-" + name)
		})
	}
}

func TestSelectAllEndsSuspendedTableRegionSession(t *testing.T) {
	h := newHarness(t, "before\n\n"+bidirectionalTableSource+"\n\nafter")
	table := documentTestTable(t, h.e)
	h.e.anchor = 0
	h.e.caret = table.tab.rows[2][0].start + 2
	h.e.modal.mode = "Visual"
	h.e.tableDocumentRegion = tableRegionSelection(table, 1, 0, 2, 0, true)
	h.e.tableDocumentRegion.fromText = true
	h.e.tableDocumentRegion.textMode = "Visual"

	h.e.SelectAll()
	if h.e.tableDocumentRegion != nil || h.e.tableSelection != nil || h.e.tableBridge != nil || len(h.e.tableNavigation) != 0 {
		t.Fatalf("SelectAll retained an earlier table selection session: region=%+v active=%+v bridge=%+v navigation=%+v", h.e.tableDocumentRegion, h.e.tableSelection, h.e.tableBridge, h.e.tableNavigation)
	}
	projected := h.e.tableStructureSelection(table)
	if projected == nil || projected.axis != tableWhole || projected.first != 0 || projected.last != len(table.tab.rows)-1 {
		t.Fatalf("SelectAll projected a stale partial region instead of the whole table: %+v", projected)
	}
}
