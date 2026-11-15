package editor

import (
	"strings"
	"testing"

	tablemodel "muse/editor/blocks/table"
	"muse/editor/internal/parser"
)

func TestTableCellRegionMatrixAndClear(t *testing.T) {
	h := newHarness(t, "| A | B | C |\n| :--- | :---: | ---: |\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	if table == nil {
		t.Fatal("table did not parse")
	}
	r := tableCellRegion{lineStart: table.lineStart, anchorRow: 1, anchorCol: 0, focusRow: 2, focusCol: 1}
	m := h.e.tableCellMatrix(table, r)
	if m == nil || len(m.Cells) != 2 || len(m.Cells[0]) != 2 || m.Cells[0][0] != "1" || m.Cells[1][1] != "5" {
		t.Fatalf("matrix=%+v", m)
	}
	if m.Alignments[0] != tablemodel.Alignment(table.tab.align[0]) || m.Alignments[1] != tablemodel.Alignment(table.tab.align[1]) {
		t.Fatalf("alignment lost: %v", m.Alignments)
	}
	if !h.e.clearTableCellRegion(table, r) {
		t.Fatal("clear did not apply")
	}
	if len(table.tab.rows) != 3 || table.tab.cols() != 3 {
		t.Fatalf("dimensions changed: rows=%d cols=%d", len(table.tab.rows), table.tab.cols())
	}
	if got := h.e.Source(); got != "| A | B | C |\n| :--- | :---: | ---: |\n|  |  | 3 |\n|  |  | 6 |" {
		t.Fatalf("clear source=%q", got)
	}
}

func TestReplaceTableCellRegionExpandsAndClearsSelectionRemainder(t *testing.T) {
	h := newHarness(t, "| A | B |\n| --- | ---: |\n| 1 | 2 |\n| 3 | 4 |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	region := tableCellRegion{lineStart: table.lineStart, anchorRow: 1, anchorCol: 0, focusRow: 1, focusCol: 1}
	matrix := &tableCellMatrix{Cells: [][]string{{"x", "y", "z"}, {"q", "r", "s"}}, Alignments: []tablemodel.Alignment{tablemodel.AlignmentLeft, tablemodel.AlignmentRight, tablemodel.AlignmentCenter}}
	if !h.e.replaceTableCellRegion(table, region, matrix) {
		t.Fatal("replace did not apply")
	}
	h.e.layoutIfNeeded()
	updated := documentTestTable(t, h.e)
	if len(updated.tab.rows) != 3 || updated.tab.cols() != 3 {
		t.Fatalf("table dimensions=%dx%d, want 3x3", len(updated.tab.rows), updated.tab.cols())
	}
	if got := h.e.Source(); got != "| A | B |  |\n| :--- | ---: | :---: |\n| x | y | z |\n| q | r | s |" {
		t.Fatalf("replace source=%q", got)
	}
}

func TestTableCellRegionCaretStaysInsideFocusCell(t *testing.T) {
	h := newHarness(t, "| A | B | C |\n| --- | --- | --- |\n| 1 | 2 | 3 |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	if !h.e.selectTableCellRegion(table, 0, 0, 1, 2) {
		t.Fatal("could not select region")
	}
	caret, ok := h.e.tableSelectionCaretRect()
	if !ok {
		t.Fatal("region caret geometry unavailable")
	}
	wantX := tableContentX(table) + table.geom.colX[2]
	if caret.X != wantX || caret.X >= tableContentX(table)+table.geom.w {
		t.Fatalf("caret x=%v want focus cell start %v within table ending %v", caret.X, wantX, tableContentX(table)+table.geom.w)
	}
}

func TestSelectTableCellRegionClearsTextSelectionFlags(t *testing.T) {
	h := newHarness(t, "| A | B |\n| --- | --- |\n| 1 | 2 |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.e.anchor, h.e.caret = table.tab.rows[1][0].start, table.tab.rows[1][1].end
	h.e.selectionInclusive = true
	h.e.selectionLinewise = true
	h.e.modal.sequence.Linewise = true
	if !h.e.selectTableCellRegion(table, 1, 0, 1, 1) {
		t.Fatal("select region failed")
	}
	if h.e.selectionInclusive || h.e.selectionLinewise || h.e.modal.sequence.Linewise {
		t.Fatalf("text selection flags remained active: inclusive=%v linewise=%v modal.line=%v", h.e.selectionInclusive, h.e.selectionLinewise, h.e.modal.sequence.Linewise)
	}
	if a, z := h.e.textSelectionRange(); a != z {
		t.Fatalf("text selection stayed active under cell region: [%d,%d)", a, z)
	}
	if s := h.e.tableSelection; s == nil || !s.cellRegion {
		t.Fatalf("cell region did not become active: %+v", s)
	}
}

func TestTableCellRegionPasteReplacesSelectionAndUndoRestores(t *testing.T) {
	const src = "| A | B | C |\n| --- | --- | --- |\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |"
	h := newHarness(t, src)
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.e.selectTableCellRegion(table, 1, 0, 2, 1)
	h.e.modal.registerTable = &tableRegister{axis: tableCells, region: true, rows: [][]string{{"x"}}, align: []parser.Align{parser.AlignCenter}}
	if !h.e.pasteTableRegister(false) {
		t.Fatal("paste into active region failed")
	}
	want := "| A | B | C |\n| :---: | --- | --- |\n| x |  | 3 |\n|  |  | 6 |"
	if h.e.Source() != want {
		t.Fatalf("region replacement=%q want %q", h.e.Source(), want)
	}
	if !h.e.Undo() || h.e.Source() != src {
		t.Fatalf("Undo did not restore source: %q", h.e.Source())
	}
	if s := h.e.tableSelection; s == nil || !s.cellRegion || s.anchorRow != 1 || s.anchorCol != 0 || s.focusRow != 2 || s.focusCol != 1 {
		t.Fatalf("Undo did not restore canonical region state: %+v", s)
	}
	if !h.e.Redo() || h.e.Source() != want {
		t.Fatalf("Redo did not restore replacement: %q", h.e.Source())
	}
	if h.e.tableSelection != nil {
		t.Fatalf("Redo retained a stale active region: %+v", h.e.tableSelection)
	}
}

func TestTableCellRegionPutBeforeAndAfterCurrentCell(t *testing.T) {
	for _, before := range []bool{true, false} {
		t.Run(map[bool]string{true: "P", false: "p"}[before], func(t *testing.T) {
			h := newHarness(t, "| A | B |\n| --- | --- |\n| 1 | 2 |")
			h.e.layoutIfNeeded()
			table := documentTestTable(t, h.e)
			h.e.caret = table.tab.rows[1][0].start
			h.e.modal.registerTable = &tableRegister{axis: tableCells, region: true, rows: [][]string{{"x", "y"}}, align: []parser.Align{parser.AlignLeft, parser.AlignRight}}
			if !h.e.pasteTableRegister(before) {
				t.Fatal("region register put failed")
			}
			want := "| A | B |  |\n| --- | :--- | ---: |\n| 1 | x | y |"
			if before {
				want = "| A | B |\n| :--- | ---: |\n| x | y |"
			}
			if h.e.Source() != want {
				t.Fatalf("put source=%q want %q", h.e.Source(), want)
			}
		})
	}
}

func TestFullHeightCellRegionKeepsColumnInsertRegisterSemantics(t *testing.T) {
	const src = "| A | B |\n| --- | ---: |\n| 1 | 2 |\n| 3 | 4 |"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.e.selectTableCellRegion(table, 0, 1, 2, 1)
	if !h.e.applyTableSelection("d") {
		t.Fatal("delete full-height column region failed")
	}
	reg := h.e.modal.registerTable
	if reg == nil || reg.axis != tableColumns || !reg.region {
		t.Fatalf("full-height selection did not become structural column register: %+v", reg)
	}
	deleted := h.e.Source()
	if !h.e.putRegister(false, 1) {
		t.Fatal("column register put failed")
	}
	putSource := h.e.Source()
	requireTableValues(t, h, [][]string{{"A", "B"}, {"1", "2"}, {"3", "4"}})
	if !h.e.Undo() || h.e.Source() != deleted {
		t.Fatalf("Undo did not restore deleted-column state: %q", h.e.Source())
	}
	if !h.e.Redo() || h.e.Source() != putSource {
		t.Fatalf("Redo did not restore structural column put: %q", h.e.Source())
	}
}

func TestFullBodyRowCellRegionKeepsStructuralPutSemantics(t *testing.T) {
	for _, before := range []bool{false, true} {
		t.Run(map[bool]string{true: "P", false: "p"}[before], func(t *testing.T) {
			const src = "| A | B |\n| --- | --- |\n| 1 | one |\n| 2 | two |\n| 3 | three |"
			h := newHarness(t, src)
			h.e.SetKeymap("vim")
			h.e.layoutIfNeeded()
			table := documentTestTable(t, h.e)
			if !h.e.selectTableCellRegion(table, 2, 0, 2, table.tab.cols()-1) {
				t.Fatal("could not select full body row")
			}
			if h.e.tableSelection.axis != tableRows {
				t.Fatalf("full row shape axis=%d, want rows", h.e.tableSelection.axis)
			}
			if !h.e.applyTableSelection("d") {
				t.Fatal("delete selected row failed")
			}
			reg := h.e.modal.registerTable
			if reg == nil || reg.axis != tableRows || !reg.region {
				t.Fatalf("full row register=%+v, want rectangular row register", reg)
			}
			deleted := h.e.Source()
			if !h.e.putRegister(before, 1) {
				t.Fatal("structural row put failed")
			}
			if got := h.e.Source(); strings.Contains(got, "|  |  |") {
				t.Fatalf("row register was pasted as blank cell matrix: %q", got)
			}
			want := [][]string{{"A", "B"}, {"1", "one"}, {"2", "two"}, {"3", "three"}}
			if !before {
				want = [][]string{{"A", "B"}, {"1", "one"}, {"3", "three"}, {"2", "two"}}
			}
			requireTableValues(t, h, want)
			putSource := h.e.Source()
			if !h.e.Undo() || h.e.Source() != deleted {
				t.Fatalf("Undo did not restore deleted-row state: %q", h.e.Source())
			}
			if !h.e.Redo() || h.e.Source() != putSource {
				t.Fatalf("Redo did not restore row put: %q", h.e.Source())
			}
		})
	}
}

func TestTableCellRegionPasteExpandsRowsPreservesCellsAndAlignment(t *testing.T) {
	const src = "| A | B |\n| --- | --- |\n| 1 | keep-1 |\n| 2 | keep-2 |"
	h := newHarness(t, src)
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.e.selectTableCellRegion(table, 2, 0, 2, 0)
	h.e.modal.registerTable = &tableRegister{
		axis: tableCells, region: true,
		rows:  [][]string{{"x", "y"}, {"z", "w"}},
		align: []parser.Align{parser.AlignCenter, parser.AlignRight},
	}
	if !h.e.pasteTableRegister(false) {
		t.Fatal("paste from last body row failed")
	}
	want := "| A | B |\n| :---: | ---: |\n| 1 | keep-1 |\n| x | y |\n| z | w |"
	if h.e.Source() != want {
		t.Fatalf("expanded paste=%q want %q", h.e.Source(), want)
	}
	if !h.e.Undo() || h.e.Source() != src {
		t.Fatalf("Undo did not restore original table: %q", h.e.Source())
	}
	if s := h.e.tableSelection; s == nil || !s.cellRegion || s.anchorRow != 2 || s.anchorCol != 0 || s.focusRow != 2 || s.focusCol != 0 {
		t.Fatalf("Undo did not restore the paste target region: %+v", s)
	}
	if !h.e.Redo() || h.e.Source() != want {
		t.Fatalf("Redo did not restore expanded table: %q", h.e.Source())
	}
	if h.e.tableSelection != nil {
		t.Fatalf("Redo retained a stale active region: %+v", h.e.tableSelection)
	}
}
