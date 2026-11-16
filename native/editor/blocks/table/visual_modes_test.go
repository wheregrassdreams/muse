package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func TestVimVisualLineInTableSelectsCurrentStructuralRow(t *testing.T) {
	const src = "| name | value |\n| --- | ---: |\n| alpha | 1 |\n| beta | 2 |\n| gamma | 3 |\n\nafter"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	cell := table.tab.rows[1][1]
	h.caretAt(cell.start)
	sendVim(h, "V")
	if h.e.tableSelection == nil || h.e.tableSelection.axis != tableRows || h.e.tableSelection.first != 1 || h.e.tableSelection.last != 1 {
		t.Fatalf("V in table cell did not select its structural row: selection=%+v bridge=%+v", h.e.tableSelection, h.e.tableBridge)
	}
	if h.e.selectionLinewise || h.e.selectionInclusive {
		t.Fatal("table structural Visual Line also retained text linewise/inclusive flags")
	}
	if got := h.e.selectionFocus(); h.e.srcLineTableOffset(got) != 1 {
		t.Fatalf("structural Visual Line focus belongs to table row %d, want 1 at offset %d", h.e.srcLineTableOffset(got), got)
	}
	if !h.e.Copy() {
		t.Fatal("structural Visual Line copy failed")
	}
	md, _ := h.clip.Read(transfer.Markdown)
	if got := string(md); got != "| name | value |\n| --- | ---: |\n| alpha | 1 |" {
		t.Fatalf("row copy is not a standalone valid table: %q", got)
	}
}

func TestVimVisualLineMovesByRenderedTableRowsAndReverses(t *testing.T) {
	h := newHarness(t, "before\n\n| name | value |\n| --- | ---: |\n| alpha | 1 |\n| beta | 2 |\n| gamma | 3 |\n\nafter")
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[1][1].start)
	sendVim(h, "V2j")
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 1 || s.last != 3 || s.anchor != 1 || s.active != 3 {
		t.Fatalf("V2j did not select successive table rows: %+v", s)
	}
	sendVim(h, "2k")
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 1 || s.last != 1 || s.anchor != 1 || s.active != 1 {
		t.Fatalf("2k did not reverse the row range to its original row: %+v", s)
	}
	if got := h.e.SelectedText(); !strings.Contains(got, "| alpha | 1 |") || strings.Contains(got, "| beta | 2 |") {
		t.Fatalf("reversed structural row selection = %q", got)
	}
}

func TestVimVisualLineKeepsRowShapeAcrossHorizontalAndWordMotions(t *testing.T) {
	for _, keys := range []string{"Vh", "Vl", "Vw", "V^", "V$"} {
		t.Run(keys, func(t *testing.T) {
			h := newHarness(t, "| name | value |\n| --- | ---: |\n| alpha beta | one two |\n| gamma | three |")
			h.e.SetKeymap("vim")
			table := documentTestTable(t, h.e)
			cell := table.tab.rows[1][0]
			h.caretAt(cell.start)
			sendVim(h, keys)
			if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 1 || s.last != 1 {
				t.Fatalf("%s shrunk the active row structure: %+v", keys, s)
			}
			if h.e.selectionLinewise || h.e.selectionInclusive {
				t.Fatalf("%s activated a competing text selection", keys)
			}
		})
	}
}

func TestVimVisualLineWordMotionAcrossRowsExpandsStructure(t *testing.T) {
	h := newHarness(t, "| name | value |\n| --- | ---: |\n| alpha | one |\n| beta | two |\n| gamma | three |")
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[1][0].start)
	sendVim(h, "Vjw")
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 1 || s.last != 2 || s.anchor != 1 || s.active != 2 {
		t.Fatalf("word motion after vertical row expansion changed the anchored row range: %+v", s)
	}
}

func TestVimVisualLineLeavesTableWithMixedVisibleFocus(t *testing.T) {
	h := newHarness(t, "before\n\n| name | value |\n| --- | ---: |\n| alpha | 1 |\n| beta | 2 |\n\nafter")
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[len(table.tab.rows)-1][1].start)
	sendVim(h, "V2j")
	if h.e.tableSelection != nil || !h.e.selectionLinewise {
		t.Fatalf("leaving table did not convert to mixed linewise selection: table=%+v linewise=%v", h.e.tableSelection, h.e.selectionLinewise)
	}
	if u := h.e.unitAt(h.e.caret); u == nil || u.table != nil {
		t.Fatalf("focus remained in table after j exit: offset=%d unit=%+v", h.e.caret, u)
	}
	if _, ok := h.e.caretRect(h.e.selectionFocus()); !ok {
		t.Fatal("leaving table lost visible linewise focus")
	}
	if got := h.e.SelectedText(); !strings.Contains(got, "| beta | 2 |") || !strings.Contains(got, "after") {
		t.Fatalf("mixed table/document selection lost a selected component: %q", got)
	}
}

func TestVimVisualLineReversesMixedTableExitToOriginalRow(t *testing.T) {
	h := newHarness(t, "before\n\n| H |\n| --- |\n| one |\n| two |\n\nafter")
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[0][0].start)
	sendVim(h, "V3j")
	if h.e.tableSelection != nil || !h.e.selectionLinewise {
		t.Fatalf("counted V motion did not leave the table into linewise document selection: table=%+v linewise=%v", h.e.tableSelection, h.e.selectionLinewise)
	}
	sendVim(h, "3k")
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 0 || s.last != 0 || s.anchor != 0 || s.active != 0 {
		t.Fatalf("reverse motion failed to restore the original header-row structure: %+v (linewise=%v)", s, h.e.selectionLinewise)
	}
	if got := h.e.SelectedText(); got != "| H |\n| --- |" {
		t.Fatalf("reverse motion did not recover original row range: %q", got)
	}
}

func TestVimCtrlVColumnKeepsStructureWhileMovingRows(t *testing.T) {
	h := newHarness(t, "| name | value |\n| --- | ---: |\n| alpha | 1 |\n| beta | 2 |\n| gamma | 3 |")
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	cell := table.tab.rows[1][1]
	if !h.e.nativeStartTableColumnVisual(cell.start) {
		t.Fatal("table column visual helper rejected a table cell")
	}
	h.e.modal.mode = "BlockVisual"
	sendVim(h, "2j")
	if s := h.e.tableSelection; s == nil || s.axis != tableColumns || s.first != 1 || s.last != 1 || s.activeRow != 3 {
		t.Fatalf("2j changed the selected table column or missed focus row: %+v", s)
	}
	sendVim(h, "2k")
	if s := h.e.tableSelection; s == nil || s.axis != tableColumns || s.first != 1 || s.last != 1 || s.activeRow != 1 {
		t.Fatalf("2k did not reverse the active row within the column: %+v", s)
	}
}

func TestVimVisualTableYAndPUseValidRowRegister(t *testing.T) {
	const src = "| name | value |\n| --- | ---: |\n| alpha | 1 |\n| beta | 2 |\n| gamma | 3 |"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[1][0].start)
	sendVim(h, "Vy")
	if h.e.modal.registerTable == nil || h.e.modal.registerTable.axis != tableRows || h.e.tableSelection != nil {
		t.Fatalf("Vy did not transfer the row structure to its table register: reg=%+v selection=%+v", h.e.modal.registerTable, h.e.tableSelection)
	}
	if html, _ := h.clip.Read(transfer.HTML); !strings.Contains(string(html), "<table>") {
		md, _ := h.clip.Read(transfer.Markdown)
		t.Fatalf("Vy clipboard is not a standalone table row: %q", md)
	}
	table = documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[len(table.tab.rows)-1][0].start)
	sendVim(h, "p")
	table = documentTestTable(t, h.e)
	if len(table.tab.rows) != 5 || h.e.src[table.tab.rows[4][0].start:table.tab.rows[4][0].end] != "alpha" {
		t.Fatalf("p did not paste a valid copied row: %q", h.e.Source())
	}
	sendVim(h, "u")
	if h.e.Source() != src {
		t.Fatalf("table row paste undo lost original source: %q", h.e.Source())
	}
}

func TestVimVisualTableDeleteThenPutRestoresSelectedRow(t *testing.T) {
	const src = "| name | value |\n| --- | ---: |\n| alpha | 1 |\n| beta | 2 |\n| gamma | 3 |"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[1][0].start)
	sendVim(h, "Vj")
	sendVim(h, "d")
	sendVim(h, "p")
	h.e.layoutIfNeeded()
	tables := 0
	for _, leaf := range h.e.leaves {
		if leaf.kind == kTable {
			tables++
			if len(leaf.tab.rows) != 4 {
				t.Fatalf("Vjd p created %d table rows, source=%q", len(leaf.tab.rows), h.e.Source())
			}
			for i, want := range []string{"gamma", "alpha", "beta"} {
				cell := leaf.tab.rows[i+1][0]
				if got := h.e.src[cell.start:cell.end]; got != want {
					t.Fatalf("pasted row %d = %q, want %q; source=%q", i, got, want, h.e.Source())
				}
			}
		}
	}
	if tables != 1 {
		t.Fatalf("Vjd p left %d tables, want one: %q", tables, h.e.Source())
	}
}

func TestVimVisualTableFullColumnDeleteThenPutKeepsStructure(t *testing.T) {
	const src = "| name | value |\n| --- | ---: |\n| alpha | 1 |\n| beta | 2 |"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[1][0].start)
	if !h.e.modalKey(ui.KeyV, ui.Ctrl) {
		t.Fatal("Ctrl+v did not enter table column selection")
	}
	h.frame()
	if s := h.e.tableSelection; s == nil || s.axis != tableCells || s.anchorRow != 1 || s.anchorCol != 0 || s.focusRow != 1 || s.focusCol != 0 {
		t.Fatalf("Ctrl+v did not start at the current cell: %+v", s)
	}
	// Extend from the first body cell through the header and the final body row,
	// producing the complete first column rectangle.
	sendVim(h, "k2j")
	if s := h.e.tableSelection; s == nil || s.axis != tableCells || s.anchorCol != 0 || s.focusCol != 0 || s.anchorRow != 1 || s.focusRow != 2 {
		t.Fatalf("motions did not create a full-height column rectangle: %+v", s)
	}
	sendVim(h, "d")
	sendVim(h, "p")
	h.e.layoutIfNeeded()
	tables := 0
	for _, leaf := range h.e.leaves {
		if leaf.kind == kTable {
			tables++
			if leaf.tab.cols() != 2 || len(leaf.tab.rows) != 3 {
				t.Fatalf("d p lost column structure: cols=%d rows=%d source=%q", leaf.tab.cols(), len(leaf.tab.rows), h.e.Source())
			}
		}
	}
	if tables != 1 {
		t.Fatalf("d p produced %d tables, want one: %q", tables, h.e.Source())
	}
}

func TestVimCtrlVColumnChangeUsesTableMultiInsertPath(t *testing.T) {
	h := newHarness(t, "| name | value |\n| --- | ---: |\n| alpha | 1 |\n| beta | 2 |")
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	if !h.e.nativeStartTableColumnVisual(table.tab.rows[1][1].start) {
		t.Fatal("table column visual helper rejected the current cell")
	}
	h.e.modal.mode = "BlockVisual"
	sendVim(h, "c")
	if h.e.modal.mode != "Insert" || h.e.modal.blockInsert == nil || h.e.block != nil {
		t.Fatalf("table c did not enter the table-specific multi-insert path: mode=%q blockInsert=%+v block=%+v", h.e.modal.mode, h.e.modal.blockInsert, h.e.block)
	}
	h.typ("x")
	table = documentTestTable(t, h.e)
	for row := range table.tab.rows {
		cell := table.tab.rows[row][1]
		if got := h.e.src[cell.start:cell.end]; got != "x" {
			t.Fatalf("table c input was not replicated in row %d, cell=%q source=%q", row, got, h.e.Source())
		}
	}
}

func TestVimCtrlVColumnLeavesTableAtVisibleEdge(t *testing.T) {
	h := newHarness(t, "before\n\n| name | value |\n| --- | ---: |\n| alpha | 1 |\n| beta | 2 |\n\nafter")
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	cell := table.tab.rows[len(table.tab.rows)-1][1]
	if !h.e.nativeStartTableColumnVisual(cell.start) {
		t.Fatal("table column visual helper rejected the current cell")
	}
	h.e.modal.mode = "BlockVisual"
	sendVim(h, "j")
	if h.e.tableSelection != nil || h.e.modal.mode != "Normal" {
		t.Fatalf("column edge did not exit structural mode cleanly: mode=%q selection=%+v", h.e.modal.mode, h.e.tableSelection)
	}
	if u := h.e.unitAt(h.e.caret); u == nil || u.table != nil {
		t.Fatalf("column edge left focus in table: offset=%d unit=%+v", h.e.caret, u)
	}
	if _, ok := h.e.caretRect(h.e.selectionFocus()); !ok {
		t.Fatal("column edge lost visible focus")
	}
}

func TestVimTableDeleteAllColumnsRemovesWholeTableAndUndoRestoresShape(t *testing.T) {
	const src = "before\n\n| name | value |\n| --- | ---: |\n| alpha | 1 |\n| beta | 2 |\n\nafter"
	h := newHarness(t, src)
	table := documentTestTable(t, h.e)
	h.e.selectTableAxis(table, tableColumns, 0, table.tab.cols()-1, false)
	if !h.e.applyTableSelection("d") {
		t.Fatal("deleting all table columns was rejected")
	}
	if strings.Contains(h.e.Source(), "| name |") || strings.Contains(h.e.Source(), "| --- |") || strings.Contains(h.e.Source(), "| alpha |") {
		t.Fatalf("deleting all columns left table delimiter fragments: %q", h.e.Source())
	}
	if !h.e.Undo() || h.e.Source() != src || h.e.tableSelection == nil || h.e.tableSelection.axis != tableColumns || h.e.tableSelection.first != 0 || h.e.tableSelection.last != 1 {
		t.Fatalf("undo did not restore the full table and column selection: source=%q selection=%+v", h.e.Source(), h.e.tableSelection)
	}
}

func TestVimCtrlVInTableSelectsCurrentCell(t *testing.T) {
	h := newHarness(t, "| name | value |\n| --- | ---: |\n| alpha | 1 |\n| beta | 2 |")
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	cell := table.tab.rows[1][1]
	h.caretAt(cell.start)
	h.key(ui.Ctrl, ui.KeyV)
	if h.e.modal.mode != "BlockVisual" || h.e.tableSelection == nil || h.e.tableSelection.axis != tableCells || h.e.tableSelection.anchorRow != 1 || h.e.tableSelection.anchorCol != 1 || h.e.tableSelection.focusRow != 1 || h.e.tableSelection.focusCol != 1 {
		t.Fatalf("Ctrl+V in cell did not start at the current cell: mode=%q selection=%+v", h.e.modal.mode, h.e.tableSelection)
	}
}

func TestVimCtrlVCellRegionExpandsInTwoDimensions(t *testing.T) {
	h := newHarness(t, "| A | B | C |\n| --- | --- | --- |\n| one | two | three |\n| four | five | six |\n| seven | eight | nine |")
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[1][1].start)
	h.key(ui.Ctrl, ui.KeyV)
	sendVim(h, "j")
	if s := h.e.tableSelection; s == nil || s.axis != tableCells || s.anchorRow != 1 || s.focusRow != 2 || s.anchorCol != 1 || s.focusCol != 1 {
		t.Fatalf("j did not extend the cell region to row 2: %+v", s)
	}
	sendVim(h, "l")
	if s := h.e.tableSelection; s == nil || s.axis != tableCells || s.anchorRow != 1 || s.focusRow != 2 || s.anchorCol != 1 || s.focusCol != 2 {
		t.Fatalf("l did not extend the same region to column 2: %+v", s)
	}
	sendVim(h, "k")
	if s := h.e.tableSelection; s == nil || s.anchorRow != 1 || s.focusRow != 1 || s.anchorCol != 1 || s.focusCol != 2 {
		t.Fatalf("k changed the anchor or horizontal extent: %+v", s)
	}
}

func (e *Editor) srcLineTableOffset(off int) int {
	_, row, _, ok := e.tableBoundaryAtOffset(off)
	if !ok {
		return -1
	}
	return row
}

func TestVimVisualHeaderRowExtendsDownAndBack(t *testing.T) {
	h := newHarness(t, "| Local file | Source page | Creator / source | License |\n| --- | --- | --- | --- |\n| attachment | source | creator | license |\n| next | next | next | next |")
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[0][1].start + 1)
	sendVim(h, "V")
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 0 || s.last != 0 {
		t.Fatalf("V on header did not select the header row: %+v", s)
	}
	sendVim(h, "j")
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 0 || s.last != 1 || s.anchor != 0 || s.active != 1 {
		t.Fatalf("Vj from header omitted the header row: %+v", s)
	}
	h.shot("header-row-visual-vj")
	sendVim(h, "k")
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 0 || s.last != 0 {
		t.Fatalf("Vk did not shrink back to the header row: %+v", s)
	}
}

func TestVimCtrlVHeaderCellExtendsIntoBodyAndBack(t *testing.T) {
	h := newHarness(t, "| A | B | C |\n| --- | --- | --- |\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |")
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[0][1].start)
	h.key(ui.Ctrl, ui.KeyV)
	sendVim(h, "j")
	if s := h.e.tableSelection; s == nil || s.anchorRow != 0 || s.focusRow != 1 || s.axis != tableCells {
		t.Fatalf("Ctrl+Vj from header omitted header cells: %+v", s)
	}
	sendVim(h, "k")
	if s := h.e.tableSelection; s == nil || s.anchorRow != 0 || s.focusRow != 0 {
		t.Fatalf("Ctrl+Vk did not return to header cell: %+v", s)
	}
}

func TestPointerPlacedHeaderCaretSelectsHeaderAndNextRow(t *testing.T) {
	h := newHarness(t, "| Local file | Source page | Creator / source | License |\n| --- | --- | --- | --- |\n| attachment | source | creator | license |\n| next | next | next | next |")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	x := table.x + (table.geom.colX[1]+table.geom.colX[2])/2
	y := table.y + (table.geom.rowY[0]+table.geom.rowY[1])/2
	row, col, inside := tableCellIndexAt(table, x, y)
	if !inside || row != 0 || col != 1 {
		t.Fatalf("header point mapped to cell (%d,%d), inside=%v", row, col, inside)
	}
	h.tt.Press(x, y-h.e.scroll.Y)
	h.tt.Release(x, y-h.e.scroll.Y)
	_, row, col = cellAt(table, h.e.caret)
	if row != 0 || col != 1 {
		t.Fatalf("pointer placed caret in table cell (%d,%d), want header (0,1), offset=%d", row, col, h.e.caret)
	}
	sendVim(h, "Vj")
	s := h.e.tableSelection
	if s == nil || s.axis != tableRows || s.first != 0 || s.last != 1 || s.anchor != 0 || s.active != 1 {
		t.Fatalf("pointer-placed header caret omitted header row from Vj: %+v focus=%d", s, h.e.selectionFocus())
	}
}
