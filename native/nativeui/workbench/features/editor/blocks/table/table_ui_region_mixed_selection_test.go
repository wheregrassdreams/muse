package editor

import (
	"muse/editor/input/vim"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func TestCellRegionLeavingTableKeepsMixedTextAndCellParts(t *testing.T) {
	h := newHarness(t, "before\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\nafter")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.e.setSessionHead(table.tab.rows[0][0].start)
	h.e.modalKey(ui.KeyV, ui.Ctrl)
	sendVim(h, "j")
	if h.e.tableSelection == nil || !h.e.tableSelection.cellRegion {
		t.Fatalf("Ctrl+V + j did not select the two-row region: %+v", h.e.tableSelection)
	}
	// Move below the final body row. The active focus leaves the table, while
	// the table intersection remains a structural part of the document range.
	sendVim(h, "j")
	if h.e.tableSelection != nil || h.e.tableBridge != nil {
		t.Fatalf("leaving the table retained active cell state: region=%+v bridge=%+v", h.e.tableSelection, h.e.tableBridge)
	}
	a, z := h.e.textSelectionRange()
	if a == z {
		t.Fatal("leaving the table did not continue document text selection")
	}
	if projected := h.e.tableStructureSelection(table); projected == nil || !projected.cellRegion {
		t.Fatalf("leaving the table lost its structural segment: text=[%d,%d) table=%+v selected=%q", a, z, projected, h.e.SelectedText())
	}
	if parts := h.e.selectionParts(); len(parts) < 2 {
		t.Fatalf("expected table and outside text parts after leaving: %+v", parts)
	}
	h.shot("mixed-selection-vim-region-left-table")
}

func TestPointerDragIntoTableSplitsTextAndCellSelection(t *testing.T) {
	h := newHarness(t, "before\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\nafter")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	var before *leaf
	for _, l := range h.e.leaves {
		if l.kind != kTable && l.lineStart == 0 {
			before = l
			break
		}
	}
	if before == nil {
		t.Fatal("before-table text leaf not found")
	}
	x := before.x + before.w - 2
	y := before.y + before.h/2
	h.e.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, Clicks: 1, X: x, Y: y})
	cell := table.tab.rows[1][0]
	h.e.input(ui.InputEvent{Kind: ui.InputPointerMove, Button: 0, X: table.x + (table.geom.colX[0]+table.geom.colX[1])/2, Y: table.y + (table.geom.rowY[1]+table.geom.rowY[2])/2})
	if a, z := h.e.selection(); a == z || z <= cell.start {
		t.Fatalf("drag did not retain a document text range through the table: [%d,%d)", a, z)
	}
	textA, textZ := h.e.textSelectionForUnit(&leaf{table: table, start: cell.start, end: cell.end})
	if textA != textZ || h.e.tableStructureSelection(table) == nil {
		t.Fatalf("table intersection did not take structural ownership: projected=%+v text=[%d,%d) selected=%q", h.e.tableStructureSelection(table), textA, textZ, h.e.SelectedText())
	}
}

func TestPointerCellDragLeavingTableKeepsMixedParts(t *testing.T) {
	h := newHarness(t, "| A | B | C | D |\n| --- | --- | --- | --- |\n| a | b | c | d |\n| e | f | g | h |\n| i | j | k | l |\n\nafter")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	cell := table.tab.rows[2][3]
	x := table.x + (table.geom.colX[3]+table.geom.colX[4])/2
	y := table.y + (table.geom.rowY[2]+table.geom.rowY[3])/2
	h.e.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, Clicks: 1, X: x, Y: y})
	x, y = table.x+(table.geom.colX[0]+table.geom.colX[1])/2, table.y+(table.geom.rowY[1]+table.geom.rowY[2])/2
	h.e.input(ui.InputEvent{Kind: ui.InputPointerMove, Button: 0, X: x, Y: y})
	if s := h.e.tableSelection; s == nil || !s.cellRegion || s.anchorRow != 2 || s.anchorCol != 3 || s.focusRow != 1 || s.focusCol != 0 {
		t.Fatalf("cross-cell drag did not start cell region: %+v", s)
	}
	want := *h.e.tableSelection
	var after *leaf
	for _, l := range h.e.leaves {
		if l.lineStart > table.lineStart && l.kind != kTable {
			after = l
		}
	}
	if after == nil {
		t.Fatal("after-table leaf not found")
	}
	h.e.input(ui.InputEvent{Kind: ui.InputPointerMove, Button: 0, X: after.x + after.w/2, Y: after.y + after.h/2})
	if h.e.tableSelection != nil {
		t.Fatalf("pointer leaving the table kept cell state: %+v", h.e.tableSelection)
	}
	if a, z := h.e.selection(); a == z {
		t.Fatal("pointer leaving table collapsed the document text range")
	}
	if projected := h.e.tableStructureSelection(table); projected == nil || !projected.cellRegion || projected.anchorRow != want.anchorRow || projected.anchorCol != want.anchorCol || projected.focusRow != want.focusRow || projected.focusCol != want.focusCol {
		t.Fatalf("pointer range lost its table region: %+v", projected)
	}
	if a, z := h.e.textSelectionForUnit(&leaf{table: table, start: cell.start, end: cell.end}); a != z {
		t.Fatal("ordinary text highlighting remained active in the table")
	}
	outerText := false
	for _, part := range h.e.selectionParts() {
		outerText = outerText || part.structure == nil
	}
	if !outerText {
		t.Fatalf("pointer range lost its outside text segment: %+v", h.e.selectionParts())
	}
	h.shot("mixed-selection-pointer-region-left-table")
}

func TestVVisualCrossCellThenLeaveTableKeepsCellProjection(t *testing.T) {
	h := newHarness(t, "before\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\nafter")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.e.setSessionHead(table.tab.rows[0][0].start)
	h.e.vimController().SetMode(vim.ModeNormal)
	sendVim(h, "v")
	for i := 0; i < 40 && (h.e.tableSelection == nil || !h.e.tableSelection.cellRegion); i++ {
		h.e.modalText("l")
	}
	if s := h.e.tableSelection; s == nil || !s.cellRegion {
		t.Fatalf("v motion did not upgrade text selection across cells: %+v", s)
	}
	// Move below the last row. The explicit region ends, but range projection
	// can reconstruct a second table selection from the document endpoints.
	h.e.modalText("j")
	h.e.modalText("j")
	if h.e.tableSelection != nil {
		t.Fatalf("leaving table kept explicit cell state: %+v", h.e.tableSelection)
	}
	if a, z := h.e.selection(); a == z {
		t.Fatal("leaving table collapsed text selection")
	}
	if projected := h.e.tableStructureSelection(table); projected == nil || !projected.cellRegion {
		t.Fatalf("v path lost table cell segment after leaving: %+v selected=%q", projected, h.e.SelectedText())
	}
}

func TestWholeTableDocumentRangeProjectsOnlyWholeTableShape(t *testing.T) {
	h := newHarness(t, "| A | B |\n| --- | --- |\n| 1 | 2 |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.e.Select(table.lineStart, blockEnd(h.e.Source(), table))
	projected := h.e.tableStructureSelection(table)
	if projected == nil || projected.axis != tableWhole {
		t.Fatalf("whole-table document range did not project whole-table shape: %+v", projected)
	}
	for _, row := range table.tab.rows {
		for _, cell := range row {
			if a, z := h.e.textSelectionForUnit(&leaf{table: table, start: cell.start, end: cell.end}); a != z {
				t.Fatalf("whole-table shape did not cover overlapping text selection: cell=[%d,%d) text=[%d,%d)", cell.start, cell.end, a, z)
			}
		}
	}
}

func TestPartialDocumentTextSelectionDoesNotProjectCellShapeOverBodyText(t *testing.T) {
	const src = "| Header A | Header B |\n| --- | --- |\n| alpha | beta |\n| gamma | delta |"
	h := newHarness(t, src)
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	body := table.tab.rows[1][0]
	// This range fully covers the header row and continues into a body cell,
	// matching the screenshot's outlined row plus highlighted cell contents.
	h.e.Select(table.lineStart, body.start+len("alpha"))
	if h.e.tableSelection != nil {
		t.Fatalf("document text selection unexpectedly became persistent cell state: %+v", h.e.tableSelection)
	}
	if projected := h.e.tableStructureSelection(table); projected != nil {
		t.Fatalf("partial document selection projected a header row over text: %+v", projected)
	}
	var bodyUnit *leaf
	for _, u := range h.e.units() {
		if u.table == table && u.start <= body.start && u.end >= body.end {
			bodyUnit = u
			break
		}
	}
	if bodyUnit == nil {
		t.Fatal("could not find rendered body-cell unit")
	}
	a, z := h.e.textSelectionForUnit(bodyUnit)
	if a == z || !strings.Contains(h.e.Source()[a:z], "alpha") {
		t.Fatalf("body text selection was lost: [%d,%d) %q", a, z, h.e.Source()[a:z])
	}
}

func TestPartialDocumentTextRangeEndingAtTableEndDoesNotProjectCells(t *testing.T) {
	h := newHarness(t, "| A | B |\n| --- | --- |\n| alpha | beta |\n| gamma | delta |\n\nafter")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	start := table.tab.rows[1][0].start + 2
	h.e.Select(start, blockEnd(h.e.Source(), table))
	if a, z := h.e.selection(); a == z {
		t.Fatal("document text range collapsed")
	}
	if projected := h.e.tableStructureSelection(table); projected != nil {
		t.Fatalf("text range ending at table edge was projected into cells: %+v", projected)
	}
	if a, z := h.e.textSelectionForUnit(&leaf{table: table, start: table.tab.rows[1][0].start, end: table.tab.rows[1][0].end}); a == z {
		t.Fatal("partial text selection through table was hidden")
	}
}

func TestCellRegionAndDormantTextRangeRemainVisuallyExclusive(t *testing.T) {
	h := newHarness(t, "| A | B | C |\n| --- | --- | --- |\n| alpha | beta | gamma |\n| delta | epsilon | zeta |")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	start := table.tab.rows[1][0].start
	h.e.setSessionHead(start)
	h.e.vimController().SetMode(vim.ModeNormal)
	sendVim(h, "v")
	for i := 0; i < 40 && h.e.tableSelection == nil; i++ {
		h.e.modalText("l")
	}
	if s := h.e.tableSelection; s == nil || !s.cellRegion {
		t.Fatalf("v + l did not promote to a cell region: %+v", s)
	}
	if a, z := h.e.textSelectionRange(); a != z {
		t.Fatalf("ordinary text range remained active under a cell region: [%d,%d)", a, z)
	}
	if a, z := h.e.selection(); a == z {
		t.Fatal("cell region did not expose its structural source range")
	}
	if got := h.e.SelectedText(); got == "alpha" {
		t.Fatalf("cell region still reported dormant text selection: %q", got)
	}
}

func TestMixedCutClearsOnlySelectedCellsAndUndoRestoresDocument(t *testing.T) {
	const src = "outside\n\n| A | B | C |\n| --- | --- | --- |\n| a | b | c |\n| d | e | f |\n\nafter"
	h := newHarness(t, src)
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	// A document range from before the table into row two projects a 2x2
	// cell rectangle while retaining the leading paragraph as a text segment.
	h.e.setSessionAnchor(0)
	h.e.setSessionHead(table.tab.rows[1][1].end)
	h.e.tableSelection = nil
	if s := h.e.tableStructureSelection(table); s == nil || !s.cellRegion || s.anchorRow != 0 || s.anchorCol != 0 || s.focusRow != 1 || s.focusCol != 1 {
		t.Fatalf("mixed range did not project its expected rectangle: %+v", s)
	}
	if !h.e.Cut() {
		t.Fatal("mixed selection cut did not copy")
	}
	markdown, err := h.clip.Read(transfer.Markdown)
	if err != nil {
		t.Fatal(err)
	}
	md := string(markdown)
	if !strings.Contains(md, "outside") || !strings.Contains(md, "| A | B |") || !strings.Contains(md, "| a | b |") || strings.Contains(md, "| C |") || strings.Contains(md, "| c |") {
		t.Fatalf("mixed clipboard did not preserve outside text and only the selected matrix: %q", md)
	}
	html, err := h.clip.Read(transfer.HTML)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), "<table>") || !strings.Contains(string(html), "outside") {
		t.Fatalf("mixed HTML clipboard lost text or table semantics: %s", html)
	}
	if strings.Contains(h.e.Source(), "outside") || strings.Contains(h.e.Source(), "| a | b |") || !strings.Contains(h.e.Source(), "|  |  | c |") || !strings.Contains(h.e.Source(), "| d | e | f |") {
		t.Fatalf("cut did not delete external text and only the selected rectangle: %q", h.e.Source())
	}
	if !h.e.Undo() || h.e.Source() != src {
		t.Fatalf("one undo did not restore the complete mixed cut: %q", h.e.Source())
	}
}

func TestMixedStructuralCutKeepsTableShapeAndUndo(t *testing.T) {
	const src = "outside\n\n| A | B | C |\n| --- | --- | --- |\n| a | b | c |\n| d | e | f |\n\nafter"
	for _, tc := range []struct {
		name   string
		region func(*leaf) *tableSelection
		check  func(*testing.T, *Editor)
	}{
		{
			name:   "row",
			region: func(t *leaf) *tableSelection { return testTableRegion(t, 1, 0, 1, 2, true) },
			check: func(t *testing.T, e *Editor) {
				if !strings.Contains(e.Source(), "| d | e | f |") || strings.Contains(e.Source(), "| a | b | c |") {
					t.Fatalf("row cut has wrong contents: %q", e.Source())
				}
			},
		},
		{
			name:   "column",
			region: func(t *leaf) *tableSelection { return testTableRegion(t, 0, 1, 2, 1, true) },
			check: func(t *testing.T, e *Editor) {
				table := documentTestTable(t, e)
				if table.tab.cols() != 2 || e.Source()[table.tab.rows[0][1].start:table.tab.rows[0][1].end] != "C" {
					t.Fatalf("column cut did not preserve remaining columns: %q", e.Source())
				}
			},
		},
		{
			name:   "whole table",
			region: func(t *leaf) *tableSelection { return testTableRegion(t, 0, 0, 2, 2, true) },
			check: func(t *testing.T, e *Editor) {
				if strings.Contains(e.Source(), "| A |") || strings.Contains(e.Source(), "| a |") {
					t.Fatalf("whole table was not removed: %q", e.Source())
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, src)
			h.e.layoutIfNeeded()
			table := documentTestTable(t, h.e)
			h.e.setSessionSelection(0, len(h.e.Source()))
			region := tc.region(table)
			h.e.tableDocumentRegion = region
			if !h.e.Cut() {
				t.Fatal("mixed structural cut did not copy")
			}
			tc.check(t, h.e)
			if strings.Contains(h.e.Source(), "outside") || strings.Contains(h.e.Source(), "after") {
				t.Fatalf("outside text was not deleted: %q", h.e.Source())
			}
			if !h.e.Undo() || h.e.Source() != src {
				t.Fatalf("one undo failed to restore source: %q", h.e.Source())
			}
			if tc.name == "row" || tc.name == "column" || tc.name == "whole table" {
				if got := h.e.tableStructureSelection(documentTestTable(t, h.e)); got == nil || got.axis != region.axis || got.anchorRow != region.anchorRow || got.anchorCol != region.anchorCol || got.focusRow != region.focusRow || got.focusCol != region.focusCol {
					t.Fatalf("undo did not restore the table region: got=%+v want=%+v", got, region)
				}
				if !h.e.Redo() || h.e.tableDocumentRegion != nil {
					t.Fatalf("redo did not restore the collapsed post-cut region state: %+v", h.e.tableDocumentRegion)
				}
			}
		})
	}
}
