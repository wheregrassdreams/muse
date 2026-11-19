package editor

import (
	"fmt"
	"muse/editor/input/vim"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
	tablemodel "muse/editor/blocks/table"
)

func TestTableStructuralRowDelete(t *testing.T) {
	h := newHarness(t, "| a | b |\n| --- | --- |\n| 1 | 2 |\n| 3 | 4 |")
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
	h.e.selectTableAxis(table, tableRows, 1, 1, false)
	if got := h.e.SelectedText(); !strings.Contains(got, "1") || strings.Contains(got, "3") {
		t.Fatalf("selected row = %q", got)
	}
	if !h.e.applyTableSelection("d") {
		t.Fatal("row delete was not applied")
	}
	if strings.Contains(h.e.Source(), "| 1 | 2 |") || !strings.Contains(h.e.Source(), "| 3 | 4 |") {
		t.Fatalf("row deletion damaged wrong source: %q", h.e.Source())
	}
}

func TestTableStructuralColumnDelete(t *testing.T) {
	h := newHarness(t, "| a | b |\n| --- | --- |\n| 1 | 2 |")
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
	h.e.selectTableAxis(table, tableColumns, 1, 1, false)
	if !h.e.applyTableSelection("d") {
		t.Fatal("column delete was not applied")
	}
	if strings.Contains(h.e.Source(), "| b |") || strings.Contains(h.e.Source(), "| 2 |") {
		t.Fatalf("column remained after delete: %q", h.e.Source())
	}
}

func TestOrphanTableHeaderRepair(t *testing.T) {
	h := newHarness(t, "\n| --- | --- |\n| 1 | 2 |")
	h.caretAt(0)
	h.typ("| ")
	if !strings.HasPrefix(h.e.Source(), "|   |   |\n| --- | --- |\n") {
		t.Fatalf("repaired source = %q", h.e.Source())
	}
	h.e.layoutIfNeeded()
	found := false
	for _, l := range h.e.leaves {
		found = found || l.kind == kTable
	}
	if !found {
		t.Fatalf("repaired source did not render as a table: %q", h.e.Source())
	}
}

func TestBlockVisualRequiresBothColumnBoundariesAndShrinksBack(t *testing.T) {
	h := newHarness(t, "| a | b |\n| --- | --- |\n| 1 | 2 |")
	h.e.SetKeymap("vim")
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
	first := table.tab.rows[0][1]
	last := table.tab.rows[len(table.tab.rows)-1][1]
	h.e.vimController().SetMode(vim.ModeBlockVisual)
	h.e.setSessionSelection(first.start, last.end)
	if !h.e.structuralSelectionForRange() || h.e.tableSelection == nil || h.e.tableSelection.axis != tableColumns || h.e.tableSelection.first != 1 {
		t.Fatalf("whole column boundaries did not promote: %+v", h.e.tableSelection)
	}
	h.e.setSessionHead(last.start)
	if h.e.structuralSelectionForRange() || h.e.tableSelection != nil {
		t.Fatalf("shrunk selection remained structural: %+v", h.e.tableSelection)
	}
}

func TestCtrlVEntersBlockVisual(t *testing.T) {
	h := newHarness(t, "| a |\n| --- |\n| b |")
	h.e.SetKeymap("vim")
	h.key(ui.Ctrl, ui.KeyV)
	if h.e.vimController().Mode() != vim.ModeBlockVisual {
		t.Fatalf("mode = %q, want BlockVisual", h.e.vimController().Mode())
	}
}

func TestColumnChangeReplicatesInsertAndBackspace(t *testing.T) {
	h := newHarness(t, "| a | b |\n| --- | --- |\n| 1 | 2 |")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	h.e.selectTableAxis(table, tableColumns, 1, 1, false)
	if !h.e.applyTableSelection("c") {
		t.Fatal("column change was not applied")
	}
	h.typ("x")
	h.typ("y")
	if !strings.Contains(h.e.Source(), "| xy |") {
		t.Fatalf("block text was not replicated: %q", h.e.Source())
	}
	h.key(0, ui.KeyBackspace)
	if strings.Contains(h.e.Source(), "xy") || strings.Count(h.e.Source(), "x") != 2 {
		t.Fatalf("block backspace did not update all rows: %q", h.e.Source())
	}
}

func TestStructuralRowAndColumnPaste(t *testing.T) {
	t.Run("row", func(t *testing.T) {
		h := newHarness(t, "| a | b |\n| --- | --- |\n| 1 | 2 |")
		h.e.layoutIfNeeded()
		var table *leaf
		for _, l := range h.e.leaves {
			if l.kind == kTable {
				table = l
				break
			}
		}
		h.e.modal.registerTable = tablemodel.NewRegister(tableRows, [][]string{{"3", "4"}}, nil, nil, false, false, false)
		h.e.setSessionHead(table.tab.rows[1][0].start)
		if !h.e.pasteTableRegister(false) {
			t.Fatal("row paste was not applied")
		}
		h.e.layoutIfNeeded()
		if got := len(h.e.leaves[0].tab.rows); got != 3 {
			t.Fatalf("rendered row count = %d, source %q", got, h.e.Source())
		}
	})
	t.Run("column", func(t *testing.T) {
		h := newHarness(t, "| a | b |\n| --- | --- |\n| 1 | 2 |")
		h.e.layoutIfNeeded()
		var table *leaf
		for _, l := range h.e.leaves {
			if l.kind == kTable {
				table = l
				break
			}
		}
		h.e.modal.registerTable = tablemodel.NewRegister(tableColumns, [][]string{{"x"}, {"y"}}, tableAlignmentsFromParser([]syntax.IncrementalAlign{syntax.IncrementalAlignCenter}), nil, false, false, false)
		h.e.setSessionHead(table.tab.rows[0][1].start)
		if !h.e.pasteTableRegister(true) {
			t.Fatal("column paste was not applied")
		}
		h.e.layoutIfNeeded()
		for _, l := range h.e.leaves {
			if l.kind == kTable {
				if l.tab.cols() != 3 {
					t.Fatalf("column count = %d, source %q", l.tab.cols(), h.e.Source())
				}
				return
			}
		}
		t.Fatalf("pasted source did not parse as table: %q", h.e.Source())
	})
}

func TestTableEdgeCaretUsesOneRowHeight(t *testing.T) {
	h := newHarness(t, "| a |\n| --- |\n| b |")
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
	left, ok := h.e.caretRect(table.lineStart)
	if !ok {
		t.Fatal("left table caret has no geometry")
	}
	right, ok := h.e.caretRect(blockEnd(h.e.Source(), table))
	if !ok {
		t.Fatal("right table caret has no geometry")
	}
	if left.H >= table.geom.rowY[len(table.geom.rowY)-1] || right.H >= table.geom.rowY[len(table.geom.rowY)-1] {
		t.Fatalf("edge carets are still table-height: left=%v right=%v tableHeight=%v", left, right, table.geom.rowY[len(table.geom.rowY)-1])
	}
}

func TestStructuralRowCaretStaysOnSelectedMiddleRow(t *testing.T) {
	h := newHarness(t, "| 左对齐 | 居中 | 右对齐 |\n| :-- | :-: | --: |\n| 甲 | 乙 | 3 |\n| 一行比较长的内容，用来看看换行 | `code` | 42 |\n| 双链 | $x^2$ | 注释 而已 |")
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
	h.e.SetKeymap("vim")
	h.e.selectTableAxis(table, tableRows, 1, 1, false)
	h.shot("table-structural-middle-row-caret")
	r, ok := h.e.tableSelectionCaretRect()
	if !ok {
		t.Fatal("middle row structural caret has no geometry")
	}
	wantY := table.y + table.geom.rowY[1]
	if r.Y != wantY || r.H != table.geom.rowY[2]-table.geom.rowY[1] || r.X != table.x+table.geom.w {
		t.Fatalf("middle row caret = %+v, want x=%v y=%v rowHeight=%v", r, table.x+table.geom.w, wantY, table.geom.rowY[2]-table.geom.rowY[1])
	}
}

func TestStructuralRowCaretMovesVertically(t *testing.T) {
	h := newHarness(t, "| a |\n| --- |\n| b |\n| c |")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	h.e.selectTableAxis(table, tableRows, 1, 1, false)
	h.e.SetKeymap("vim")
	h.e.modalText("j")
	if h.e.tableSelection == nil || h.e.tableSelection.active != 2 || h.e.tableSelection.first != 2 {
		t.Fatalf("j did not move structural selection: %+v", h.e.tableSelection)
	}
	h.e.vimController().SetMode(vim.ModeVisual)
	h.e.modalText("k")
	if h.e.tableSelection == nil || h.e.tableSelection.first != 1 || h.e.tableSelection.last != 2 {
		t.Fatalf("visual k did not extend structural selection: %+v", h.e.tableSelection)
	}
}

func TestVisualStructuralRowMotionUsesTableFocus(t *testing.T) {
	h := newHarness(t, "| a |\n| --- |\n| b |\n| c |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.e.SetKeymap("vim")
	h.e.selectTableAxis(table, tableRows, 2, 2, false)
	h.e.vimController().SetMode(vim.ModeVisual)

	if !h.e.modalText("k") {
		t.Fatal("visual k was not handled")
	}
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 1 || s.last != 2 || s.anchor != 2 || s.active != 1 {
		t.Fatalf("visual k did not extend from the structural table focus: %+v", s)
	}
}

func TestCaretAtBothRowBoundariesAndVerticalMovement(t *testing.T) {
	h := newHarness(t, "| a |\n| --- |\n| b |\n| c |")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	lines := h.e.tableLines(table)
	rowLine := lines[2]
	for _, tc := range []struct {
		name  string
		off   int
		x     float32
		right bool
	}{
		{name: "left", off: rowLine[0], x: table.x},
		{name: "right", off: rowLine[1], x: table.x + table.geom.w, right: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, ok := h.e.caretRect(tc.off)
			if !ok || r.X != tc.x || r.Y != table.y+table.geom.rowY[1] {
				t.Fatalf("boundary caret=%+v ok=%v, want x=%v rowY=%v", r, ok, tc.x, table.y+table.geom.rowY[1])
			}
			h.e.setSessionSelection(tc.off, tc.off)
			h.e.vertical(1, false)
			moved, ok := h.e.caretRect(h.e.Caret())
			if !ok || moved.X != tc.x || moved.Y != table.y+table.geom.rowY[2] {
				t.Fatalf("vertical move lost boundary side: rect=%+v caret=%d", moved, h.e.Caret())
			}
		})
	}
}

func TestHorizontalStepFromUpperLeftEntersFirstCell(t *testing.T) {
	h := newHarness(t, "| a | b |\n| --- | --- |\n| 1 | 2 |")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	got := h.e.step(table.lineStart, 1, false)
	want := table.tab.rows[0][0].start
	if got != want {
		t.Fatalf("right from upper-left boundary landed at %d, want first cell start %d", got, want)
	}
}

func TestHorizontalMovementFromColumnSelectionReturnsToContent(t *testing.T) {
	h := newHarness(t, "| a | b | c |\n| --- | --- | --- |\n| 1 | 2 | 3 |")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	h.e.selectTableAxis(table, tableColumns, 0, 0, false)
	h.e.SetKeymap("vim")
	h.e.vimController().SetMode(vim.ModeVisual)
	before := h.e.selectionFocus()
	h.e.modalText("h")
	if h.e.tableSelection != nil || h.e.Caret() >= before || h.e.Session.Selection().Anchor == h.e.Caret() {
		t.Fatalf("horizontal motion did not reveal content selection: structure=%+v anchor=%d caret=%d", h.e.tableSelection, h.e.Session.Selection().Anchor, h.e.Caret())
	}
}

func TestHorizontalStructureTransitionShowsContentBeforeNextStructure(t *testing.T) {
	h := newHarness(t, "| a | b | c |\n| --- | --- | --- |\n| 1 | 2 | 3 |")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	h.e.SetKeymap("vim")
	h.e.vimController().SetMode(vim.ModeBlockVisual)
	h.e.selectTableAxis(table, tableColumns, 0, 0, false)
	if !h.e.modalText("l") {
		t.Fatal("first horizontal move was not handled")
	}
	if h.e.tableSelection != nil || h.e.tableBridge == nil || !strings.Contains(h.e.SelectedText(), "a") || !strings.Contains(h.e.SelectedText(), "b") {
		t.Fatalf("structure did not downgrade to one content selection: structure=%+v bridge=%+v selected=%q", h.e.tableSelection, h.e.tableBridge, h.e.SelectedText())
	}
	if !h.e.modalText("l") {
		t.Fatal("second horizontal move was not handled")
	}
	if h.e.tableBridge != nil || h.e.tableSelection == nil || h.e.tableSelection.axis != tableColumns || h.e.tableSelection.first != 0 || h.e.tableSelection.last != 1 {
		t.Fatalf("adjacent structure was not selected after its content: %+v", h.e.tableSelection)
	}
}

func TestHorizontalMotionFromRowStructureDoesNotPromoteColumn(t *testing.T) {
	h := newHarness(t, "| a | b |\n| --- | --- |\n| 1 | 2 |\n| 3 | 4 |")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	h.e.SetKeymap("vim")
	h.e.vimController().SetMode(vim.ModeVisual)
	h.e.selectTableAxis(table, tableRows, 1, 1, false)
	if !h.e.modalText("l") {
		t.Fatal("first horizontal motion was not handled")
	}
	if h.e.tableSelection != nil || h.e.tableBridge == nil || h.e.tableBridge.axis != tableRows || h.e.tableBridge.row != 2 {
		t.Fatalf("row motion entered an invalid bridge: %+v %+v", h.e.tableSelection, h.e.tableBridge)
	}
	if !h.e.modalText("l") {
		t.Fatal("second horizontal motion was not handled")
	}
	if h.e.tableSelection != nil && h.e.tableSelection.axis == tableColumns {
		t.Fatalf("row motion incorrectly promoted a column: %+v", h.e.tableSelection)
	}
}

func TestRowStructureLeftMovesToPreviousRowContentThenPromotes(t *testing.T) {
	h := newHarness(t, "| 头甲 | 头乙 |\n| --- | --- |\n| 甲 | 乙 |\n| 1 | 2 |")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	h.e.SetKeymap("vim")
	h.e.vimController().SetMode(vim.ModeVisual)
	h.e.selectTableAxis(table, tableRows, 2, 2, false)
	h.e.tableSelection.right = false
	a, z, ok := h.e.tableSelectionSourceRange(table, h.e.tableSelection)
	if !ok {
		t.Fatal("left-oriented row selection has no source range")
	}
	h.e.setSessionSelection(z, a)
	if !h.e.modalText("h") {
		t.Fatal("left did not leave selected row")
	}
	if h.e.tableSelection != nil || h.e.tableBridge == nil || h.e.tableBridge.axis != tableRows || h.e.tableBridge.row != 1 || !strings.Contains(h.e.SelectedText(), "乙") || !strings.Contains(h.e.SelectedText(), "1") {
		t.Fatalf("did not enter previous row content: selection=%+v bridge=%+v text=%q", h.e.tableSelection, h.e.tableBridge, h.e.SelectedText())
	}
	h.shot("row-structure-content-bridge")
	for i := 0; i < 20 && (h.e.tableSelection == nil || h.e.tableSelection.first != 1 || h.e.tableSelection.last != 2); i++ {
		h.e.modalText("h")
	}
	if h.e.tableSelection == nil || h.e.tableSelection.axis != tableRows || h.e.tableSelection.first != 1 || h.e.tableSelection.last != 2 {
		t.Fatalf("reaching previous row's left boundary did not extend the row selection: %+v caret=%d", h.e.tableSelection, h.e.Caret())
	}
}

func TestEscapeClearsVisualStructuralSelection(t *testing.T) {
	h := newHarness(t, "| a |\n| --- |\n| b |")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	h.e.selectTableAxis(table, tableRows, 1, 1, false)
	h.e.vimController().SetMode(vim.ModeVisual)
	h.key(0, ui.KeyEscape)
	if h.e.vimController().Mode() != vim.ModeNormal || h.e.tableSelection != nil || h.e.Session.Selection().Anchor != h.e.Caret() {
		t.Fatalf("Escape left selection active: mode=%q selection=%+v anchor=%d caret=%d", h.e.vimController().Mode(), h.e.tableSelection, h.e.Session.Selection().Anchor, h.e.Caret())
	}
}

func TestTableBoundaryVerticalMovementLeavesTable(t *testing.T) {
	h := newHarness(t, "before\n\n| a |\n| --- |\n| b |\n\nafter")
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
	lines := h.e.tableLines(table)
	h.e.setSessionSelection(lines[0][0], lines[0][0])
	h.e.vertical(-1, false)
	if _, _, _, boundary := h.e.tableBoundaryAtOffset(h.e.Caret()); boundary {
		t.Fatalf("up stayed on table boundary at %d", h.e.Caret())
	}
	last := len(lines) - 1
	h.e.setSessionSelection(lines[last][1], lines[last][1])
	h.e.vertical(1, false)
	if _, _, _, boundary := h.e.tableBoundaryAtOffset(h.e.Caret()); boundary {
		t.Fatalf("down stayed on table boundary at %d", h.e.Caret())
	}
	if h.e.Caret() <= lines[last][1] {
		t.Fatalf("down did not leave the table: caret=%d tableRowEnd=%d source=%q", h.e.Caret(), lines[last][1], h.e.Source())
	}
}

func TestLastTableRowSelectionDoesNotIncludeFollowingBlock(t *testing.T) {
	h := newHarness(t, "| a |\n| --- |\n| b |\n\n## 后续内容")
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
	h.e.selectTableAxis(table, tableRows, len(table.tab.rows)-1, len(table.tab.rows)-1, false)
	_, end := h.e.selection()
	lastLine := eTableLastLineEnd(h.e, table)
	if end != lastLine {
		t.Fatalf("structural selection ends at %d, want table row end %d", end, lastLine)
	}
	if end < len(h.e.Source()) && h.e.Source()[end] != '\n' {
		t.Fatalf("row end points into following content: %q", h.e.Source()[end:])
	}
}

func eTableLastLineEnd(e *Editor, table *leaf) int {
	lines := e.tableLines(table)
	return lines[len(lines)-1][1]
}

const selectionRegressionTable = "| 标题甲 | 标题乙 | 标题丙 |\n| --- | --- | --- |\n| 甲 | 一段用于观察单元格布局的较长文本 | 17 |\n| 乙 | `inline code` | 2048 |\n| 丙 | 另一段较长的中文内容用于观察换行 | 3.14 |"

func tableSelectionState(e *Editor) string {
	a, z := e.SelectionRange().Start, e.SelectionRange().End
	return fmt.Sprintf("mode=%s range=[%d,%d) selected=%q table=%+v bridge=%+v anchor=%d active=%d", e.vimController().Mode(), a, z, e.SelectedText(), e.tableSelection, e.tableBridge, e.Session.Selection().Anchor, e.Caret())
}

func TestRowVisualSelectionReversesAcrossContentBridge(t *testing.T) {
	h := newHarness(t, selectionRegressionTable)
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	if table == nil || len(table.tab.rows) < 4 {
		t.Fatal("three body row regression table did not parse")
	}
	h.e.SetKeymap("vim")
	h.e.vimController().SetMode(vim.ModeVisual)
	h.e.selectTableAxis(table, tableRows, 1, 1, false)
	h.shot("table-row-visual-single-row")

	step := func(key string) {
		t.Helper()
		before := tableSelectionState(h.e)
		if !h.e.modalText(key) {
			t.Fatalf("%q was not handled; before: %s", key, before)
		}
		t.Logf("key %q\nbefore: %s\nafter:  %s", key, before, tableSelectionState(h.e))
		if h.e.tableSelection != nil && h.e.tableBridge != nil {
			t.Fatalf("structure and content bridge were active together after %q: %s", key, tableSelectionState(h.e))
		}
	}
	assertBridge := func(row int) {
		t.Helper()
		if h.e.tableSelection != nil || h.e.tableBridge == nil || h.e.tableBridge.axis != tableRows || h.e.tableBridge.row != row {
			t.Fatalf("want row %d content bridge, got %s", row, tableSelectionState(h.e))
		}
		if a, z := h.e.selection(); a == z || h.e.SelectedText() != h.e.Source()[a:z] {
			t.Fatalf("bridge is not one ordinary source selection: %s", tableSelectionState(h.e))
		}
	}
	assertTwoRows := func() {
		t.Helper()
		if h.e.tableBridge != nil || h.e.tableSelection == nil || h.e.tableSelection.axis != tableRows || h.e.tableSelection.first != 1 || h.e.tableSelection.last != 2 {
			t.Fatalf("want structural rows [1,2], got %s", tableSelectionState(h.e))
		}
		a, z, ok := h.e.tableSelectionSourceRange(table, h.e.tableSelection)
		x, y := h.e.selection()
		if !ok || x != a || y != z || !strings.Contains(h.e.SelectedText(), "17") || !strings.Contains(h.e.SelectedText(), "2048") {
			t.Fatalf("structural selection/source range disagree: %s", tableSelectionState(h.e))
		}
	}

	// Extend right through row 2 content to the edge, promoting rows 1-2.
	step("l")
	assertBridge(2)
	h.shot("table-row-bridge-next-row")
	for i := 0; i < 100 && h.e.tableSelection == nil; i++ {
		step("l")
	}
	assertTwoRows()
	h.shot("table-row-structure-two-rows-rightward")

	// Reverse the promotion with h and restore the exact row 2 bridge state.
	// Further h motion crosses row 2 content and then shrinks to row 1.
	step("h")
	assertBridge(2)
	h.shot("table-row-bridge-reversed")
	for i := 0; i < 100 && h.e.tableSelection == nil; i++ {
		step("h")
	}
	if h.e.tableSelection == nil || h.e.tableSelection.first != 1 || h.e.tableSelection.last != 1 || !h.e.tableSelection.right {
		t.Fatalf("reverse h did not restore the original row 1 boundary: %s", tableSelectionState(h.e))
	}
	h.shot("table-row-structure-shrunk-to-row-one")
	step("l")
	assertBridge(2)
	for i := 0; i < 100 && h.e.tableSelection == nil; i++ {
		step("l")
	}
	assertTwoRows()
	h.shot("table-row-structure-two-rows-restored")
}

func TestTableInlineCodeWrappingLayoutScreenshot(t *testing.T) {
	h := newHarness(t, selectionRegressionTable)
	h.e.layoutIfNeeded()
	// Keep this capture independent from any selection-state assertions.
	h.shot("table-inline-code-wrapping-layout")
}

const rightBoundaryLeftMotionTable = "| 名称 | 内容 | 数值 |\n| --- | --- | --- |\n| 上一行 | 普通文本 | 3 |\n| 当前行 | 一段较长文本以及 `inline code` | 42 |\n| 下一行 | 其他内容 | 100 |"

func TestRowRightBoundaryLeftMotionStaysOnCurrentRow(t *testing.T) {
	h := newHarness(t, rightBoundaryLeftMotionTable)
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	if table == nil || len(table.tab.rows) < 4 {
		t.Fatal("right-boundary regression table did not parse")
	}
	h.e.SetKeymap("vim")
	h.e.vimController().SetMode(vim.ModeVisual)
	h.e.selectTableAxis(table, tableRows, 2, 2, false)
	caretRow := func() int {
		lines := h.e.tableLines(table)
		for row := range table.tab.rows {
			line := row
			if row > 0 {
				line++
			}
			if line < len(lines) && h.e.selectionFocus() >= lines[line][0] && h.e.selectionFocus() <= lines[line][1] {
				return row
			}
		}
		return -1
	}
	if caretRow() != 2 {
		t.Fatalf("fixture setup did not put the caret on body row 2: %s", tableSelectionState(h.e))
	}
	h.t.Logf("right-boundary repro before h: caretRow=%d %s", caretRow(), tableSelectionState(h.e))
	h.shot("table-row-right-boundary-before-h")

	if !h.e.modalText("h") {
		t.Fatal("left motion from right boundary was not handled")
	}
	h.t.Logf("right-boundary repro after h: caretRow=%d %s", caretRow(), tableSelectionState(h.e))
	h.shot("table-row-right-boundary-after-h")
	if got := caretRow(); got != 2 {
		t.Fatalf("h from row 2 right boundary jumped to row %d; want caret to stay/return on row 2: %s", got, tableSelectionState(h.e))
	}
	if h.e.tableSelection != nil || h.e.tableBridge == nil || h.e.tableBridge.axis != tableRows || h.e.tableBridge.row != 2 {
		t.Fatalf("h did not enter row 2 content from its right boundary: %s", tableSelectionState(h.e))
	}
	if a, z := h.e.selection(); a == z || h.e.SelectedText() != h.e.Source()[a:z] {
		t.Fatalf("current-row bridge is not one ordinary source selection: %s", tableSelectionState(h.e))
	}
}

func TestBlockVisualColumnSelectionExpandsAndReverses(t *testing.T) {
	h := newHarness(t, selectionRegressionTable)
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
	h.e.SetKeymap("vim")
	h.key(ui.Ctrl, ui.KeyV)
	h.e.selectTableAxis(table, tableColumns, 1, 1, false)
	h.e.vimController().SetMode(vim.ModeBlockVisual)
	h.shot("table-column-block-visual-one-column")
	if !h.e.modalText("l") {
		t.Fatal("rightward block visual movement was not handled")
	}
	for i := 0; i < 100 && h.e.tableBridge != nil; i++ {
		if !h.e.modalText("l") {
			t.Fatal("rightward block visual continuation was not handled")
		}
	}
	if h.e.tableSelection == nil || h.e.tableSelection.axis != tableColumns || h.e.tableSelection.first != 1 || h.e.tableSelection.last != 2 {
		t.Fatalf("rightward block selection did not include adjacent column: %s", tableSelectionState(h.e))
	}
	h.shot("table-column-block-visual-expanded-right")
	if !h.e.modalText("h") || h.e.tableSelection != nil || h.e.tableBridge == nil || h.e.tableBridge.axis != tableColumns {
		t.Fatalf("leftward reversal did not enter column content: %s", tableSelectionState(h.e))
	}
	h.shot("table-column-block-visual-content-bridge")
}

type reversibleSelectionSnapshot struct {
	mode                         string
	origin                       tablemodel.SelectionOrigin
	textPaintStart, textPaintEnd int
	focus                        int
	anchor, caret                int
	rangeStart, rangeEnd         int
	selectedText                 string
	hasTableSelection            bool
	tableLineStart               int
	tableAxis                    tableAxis
	tableFirst, tableLast        int
	tableAnchor, tableActive     int
	tableActiveRow               int
	tableRight                   bool
	hasBridge                    bool
	bridgeLineStart              int
	bridgeAxis                   tableAxis
	bridgeRow, bridgeCol         int
	bridgeDir                    int
	bridgeResume                 string
	bridgeIndex                  int
	bridgeSelectionAnchor        int
	caretLeaf                    string
	caretRow, caretCol           int
	caretCell                    int
}

func assertSelectionVariant(t *testing.T, e *Editor) {
	t.Helper()
	a, z := e.textSelectionRange()
	if e.tableSelection != nil && (e.tableBridge != nil || a != z) {
		t.Fatal("text paint and structure active together")
	}
	if e.tableSelection == nil {
		x, y := e.selection()
		if a != x || z != y {
			t.Fatal("text variant not painted")
		}
	}
}

func snapshotSelection(e *Editor, table *leaf) reversibleSelectionSnapshot {
	a, z := e.selection()
	s := reversibleSelectionSnapshot{
		mode: string(e.vimController().Mode()), anchor: e.Session.Selection().Anchor, caret: e.Caret(),
		rangeStart: a, rangeEnd: z, selectedText: e.SelectedText(), caretRow: -1, caretCol: -1, caretCell: -1,
	}
	s.focus = e.selectionFocus()
	if e.tableSelection != nil {
		s.origin = e.tableSelection.origin
	}
	if e.tableBridge != nil {
		s.origin = e.tableBridge.origin
	}
	s.textPaintStart, s.textPaintEnd = e.textSelectionRange()
	lines := e.tableLines(table)
	for row := range table.tab.rows {
		line := row
		if row > 0 {
			line++
		}
		if line < len(lines) && s.focus >= lines[line][0] && s.focus <= lines[line][1] {
			s.caretRow = row
			s.caretLeaf = "table"
			for col, cell := range table.tab.rows[row] {
				if s.focus >= cell.start && s.focus <= cell.end {
					s.caretCol, s.caretCell = col, col
					break
				}
			}
			break
		}
	}
	if s.caretLeaf == "" {
		for _, l := range e.leaves {
			if s.focus >= l.start && s.focus <= l.end {
				s.caretLeaf = fmt.Sprintf("%d:[%d,%d)", l.kind, l.start, l.end)
				break
			}
		}
	}
	if ts := e.tableSelection; ts != nil {
		s.hasTableSelection = true
		s.tableLineStart = ts.lineStart
		s.tableAxis, s.tableFirst, s.tableLast = ts.axis, ts.first, ts.last
		s.tableAnchor, s.tableActive, s.tableRight = ts.anchor, ts.active, ts.right
		s.tableActiveRow = ts.activeRow
	}
	if b := e.tableBridge; b != nil {
		s.hasBridge = true
		s.bridgeLineStart, s.bridgeAxis = b.lineStart, b.axis
		s.bridgeRow, s.bridgeCol, s.bridgeDir = b.row, b.col, b.dir
		s.bridgeResume, s.bridgeIndex = b.resume, b.index
		s.bridgeSelectionAnchor = b.selectionAnchor
	}
	return s
}

func bridgeAtHorizontalEdge(e *Editor, table *leaf) bool {
	b := e.tableBridge
	if b == nil || b.row < 0 || b.row >= len(table.tab.rows) {
		return false
	}
	if b.axis == tableRows {
		row := table.tab.rows[b.row]
		if b.dir > 0 {
			return e.Caret() == row[len(row)-1].end
		}
		return e.Caret() == row[0].start
	}
	if b.col < 0 || b.col >= len(table.tab.rows[b.row]) {
		return false
	}
	cell := table.tab.rows[b.row][b.col]
	if b.dir > 0 {
		return e.Caret() == cell.end
	}
	return e.Caret() == cell.start
}

func TestVisualContentSelectionSurvivesStructuralPromotionRoundTrip(t *testing.T) {
	const src = "| Name | Details | Count |\n| --- | --- | --- |\n| first-column-content-midpoint | enough adjacent content to cross | 17 |\n| next-row-content | more content for the table | 2048 |"
	h := newHarness(t, src)
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	if table == nil || len(table.tab.rows) < 3 || len(table.tab.rows[1]) < 2 {
		t.Fatal("content-selection table did not parse")
	}
	h.e.SetKeymap("vim")
	h.e.vimController().SetMode(vim.ModeVisual)
	firstCell := table.tab.rows[1][0]
	h.e.setSessionAnchor(firstCell.start + 6)
	h.e.setSessionHead(firstCell.start + 11)
	initial := snapshotSelection(h.e, table)
	if initial.hasTableSelection || initial.hasBridge || initial.anchor == initial.caret {
		t.Fatalf("fixture did not start with an ordinary content selection: %+v", initial)
	}

	forwardMoves := 0
	for forwardMoves < 100 && h.e.tableSelection == nil {
		if !h.e.modalText("l") {
			t.Fatalf("rightward motion %d was not handled; state=%+v", forwardMoves+1, snapshotSelection(h.e, table))
		}
		forwardMoves++
	}
	if h.e.tableSelection == nil {
		t.Fatalf("rightward motion never promoted the content selection to a structure; initial=%+v final=%+v", initial, snapshotSelection(h.e, table))
	}
	promoted := snapshotSelection(h.e, table)
	t.Logf("content selection promoted after %d l motions\ninitial: %+v\npromoted: %+v", forwardMoves, initial, promoted)

	for i := 0; i < forwardMoves; i++ {
		if !h.e.modalText("h") {
			t.Fatalf("leftward reverse motion %d/%d was not handled; state=%+v", i+1, forwardMoves, snapshotSelection(h.e, table))
		}
	}
	reversed := snapshotSelection(h.e, table)
	t.Logf("content selection after reversing %d motions: %+v", forwardMoves, reversed)
	if reversed != initial {
		t.Fatalf("reverse path did not restore the initial content selection\ninitial: %+v\nafter:   %+v", initial, reversed)
	}
}

func TestHorizontalPromotionReversesExactlyOneStep(t *testing.T) {
	for _, tc := range []struct {
		name      string
		startRow  int
		forward   string
		reverse   string
		bridgeRow int
	}{
		{name: "right then left", startRow: 1, forward: "l", reverse: "h", bridgeRow: 2},
		{name: "left then right", startRow: 2, forward: "h", reverse: "l", bridgeRow: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, selectionRegressionTable)
			h.e.layoutIfNeeded()
			var table *leaf
			for _, l := range h.e.leaves {
				if l.kind == kTable {
					table = l
					break
				}
			}
			if table == nil || len(table.tab.rows) < 4 {
				t.Fatal("reversible-selection table did not parse")
			}
			h.e.SetKeymap("vim")
			h.e.vimController().SetMode(vim.ModeVisual)
			h.e.selectTableAxis(table, tableRows, tc.startRow, tc.startRow, false)
			if tc.forward == "h" {
				h.e.tableSelection.right = false
				a, z, ok := h.e.tableSelectionSourceRange(table, h.e.tableSelection)
				if !ok {
					t.Fatal("left-oriented initial selection has no source range")
				}
				h.e.setSessionSelection(z, a)
			}

			move := func(key string) reversibleSelectionSnapshot {
				t.Helper()
				before := snapshotSelection(h.e, table)
				if !h.e.modalText(key) {
					t.Fatalf("key %q was not handled; before=%+v", key, before)
				}
				after := snapshotSelection(h.e, table)
				t.Logf("key=%q before=%+v after=%+v", key, before, after)
				return after
			}

			state := move(tc.forward) // enter adjacent row content
			if !state.hasBridge || state.bridgeAxis != tableRows || state.bridgeRow != tc.bridgeRow || state.hasTableSelection {
				t.Fatalf("initial horizontal move did not enter the expected content row: %+v", state)
			}
			promoted := false
			for i := 0; i < 100; i++ {
				if !bridgeAtHorizontalEdge(h.e, table) {
					state = move(tc.forward)
					continue
				}
				beforePromotion := snapshotSelection(h.e, table)
				t.Logf("promotion boundary snapshot=%+v", beforePromotion)
				state = move(tc.forward)
				if !state.hasTableSelection || state.hasBridge {
					t.Fatalf("forward boundary motion did not produce a pure structural selection: %+v", state)
				}
				promoted = true
				reversed := move(tc.reverse)
				if reversed != beforePromotion {
					t.Fatalf("opposite %q did not exactly reverse the promotion step\nbefore promotion: %+v\nafter reversal:   %+v", tc.reverse, beforePromotion, reversed)
				}
				break
			}
			if !promoted {
				t.Fatalf("did not reach structural promotion boundary; final state=%+v", snapshotSelection(h.e, table))
			}
		})
	}
}

func TestHorizontalColumnPromotionReversesExactlyOneStep(t *testing.T) {
	for _, tc := range []struct {
		name      string
		startCol  int
		forward   string
		reverse   string
		bridgeCol int
	}{
		{name: "right then left", startCol: 1, forward: "l", reverse: "h", bridgeCol: 2},
		{name: "left then right", startCol: 2, forward: "h", reverse: "l", bridgeCol: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, selectionRegressionTable)
			h.e.layoutIfNeeded()
			var table *leaf
			for _, l := range h.e.leaves {
				if l.kind == kTable {
					table = l
					break
				}
			}
			if table == nil || len(table.tab.rows) < 2 || len(table.tab.rows[0]) < 3 {
				t.Fatal("column reversible-selection table did not parse")
			}
			h.e.SetKeymap("vim")
			h.key(ui.Ctrl, ui.KeyV)
			if h.e.vimController().Mode() != vim.ModeBlockVisual {
				t.Fatalf("Ctrl+V mode = %q, want BlockVisual", h.e.vimController().Mode())
			}
			h.e.selectTableAxis(table, tableColumns, tc.startCol, tc.startCol, false)
			h.e.vimController().SetMode(vim.ModeBlockVisual)
			if tc.forward == "h" {
				h.e.tableSelection.right = false
				a, z, ok := h.e.tableSelectionSourceRange(table, h.e.tableSelection)
				if !ok {
					t.Fatal("left-oriented initial column selection has no source range")
				}
				h.e.setSessionSelection(z, a)
			}

			move := func(key string) reversibleSelectionSnapshot {
				t.Helper()
				before := snapshotSelection(h.e, table)
				if !h.e.modalText(key) {
					t.Fatalf("key %q was not handled; before=%+v", key, before)
				}
				after := snapshotSelection(h.e, table)
				t.Logf("key=%q before=%+v after=%+v", key, before, after)
				return after
			}

			state := move(tc.forward)
			if !state.hasBridge || state.bridgeAxis != tableColumns || state.bridgeCol != tc.bridgeCol || state.hasTableSelection {
				t.Fatalf("initial horizontal move did not enter adjacent column content: %+v", state)
			}
			promoted := false
			for i := 0; i < 100; i++ {
				if !bridgeAtHorizontalEdge(h.e, table) {
					state = move(tc.forward)
					continue
				}
				beforePromotion := snapshotSelection(h.e, table)
				t.Logf("column promotion boundary snapshot=%+v", beforePromotion)
				state = move(tc.forward)
				if !state.hasTableSelection || state.hasBridge || state.tableAxis != tableColumns || state.tableFirst != 1 || state.tableLast != 2 {
					t.Fatalf("forward boundary motion did not promote adjacent columns: %+v", state)
				}
				promoted = true
				reversed := move(tc.reverse)
				if reversed != beforePromotion {
					t.Fatalf("opposite %q did not exactly reverse the column promotion step\nbefore promotion: %+v\nafter reversal:   %+v", tc.reverse, beforePromotion, reversed)
				}
				break
			}
			if !promoted {
				t.Fatalf("did not reach column structural promotion boundary; final state=%+v", snapshotSelection(h.e, table))
			}
		})
	}
}

func TestRowStructureLeftFromHeaderAndFirstBodyRowRecordsFocusPath(t *testing.T) {
	h := newHarness(t, "| Name | Details | Count |\n| --- | --- | --- |\n| first | body content | 17 |\n| second | more content | 2048 |")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	if table == nil || len(table.tab.rows) < 3 {
		t.Fatal("header and body row fixture did not parse")
	}
	h.e.SetKeymap("vim")
	h.e.vimController().SetMode(vim.ModeVisual)
	// Approximate the screenshot's header+first-body-row structure with its
	// focus at the upper/right traversal edge, then move left from there.
	h.e.selectTableAxis(table, tableRows, 0, 1, false)
	h.e.tableSelection.right = true
	a, z, ok := h.e.tableSelectionSourceRange(table, h.e.tableSelection)
	if !ok {
		t.Fatal("initial two-row structure has no source range")
	}
	h.e.setSessionSelection(a, z)
	initial := snapshotSelection(h.e, table)
	if !initial.hasTableSelection || initial.tableAxis != tableRows || initial.tableFirst != 0 || initial.tableLast != 1 || initial.caretRow != 1 {
		t.Fatalf("fixture does not approximate left-focused header+body structure: %+v", initial)
	}
	h.shot("table-header-body-upper-right-initial")

	reachedHeaderOnly := false
	for i := 0; i < 100; i++ {
		before := snapshotSelection(h.e, table)
		if !h.e.modalText("h") {
			t.Fatalf("left motion %d was not handled; state=%+v", i+1, before)
		}
		after := snapshotSelection(h.e, table)
		t.Logf("path A h[%d] before=%+v after=%+v", i+1, before, after)
		h.shot(fmt.Sprintf("table-header-body-upper-right-h-%02d", i+1))
		if after.hasTableSelection && after.tableAxis == tableRows && after.tableFirst == 0 && after.tableLast == 0 {
			reachedHeaderOnly = true
			break
		}
	}
	t.Logf("path A reached header-only structural selection=%v final=%+v", reachedHeaderOnly, snapshotSelection(h.e, table))

	// A one-step h/l round trip must preserve every field in the complete
	// selection snapshot, including bridge state and the focused table cell.
	for _, tc := range []struct {
		first, last, active int
		right               bool
	}{
		{first: 0, last: 1, active: 1, right: true},
		{first: 1, last: 1, active: 1, right: false},
	} {
		h.e.selectTableAxis(table, tableRows, tc.first, tc.last, false)
		h.e.tableSelection.active = tc.active
		h.e.tableSelection.right = tc.right
		a, z, ok := h.e.tableSelectionSourceRange(table, h.e.tableSelection)
		if !ok {
			t.Fatal("round-trip row structure has no source range")
		}
		if tc.right {
			h.e.setSessionSelection(a, z)
		} else {
			h.e.setSessionSelection(z, a)
		}
		want := snapshotSelection(h.e, table)
		if !h.e.modalText("h") || !h.e.modalText("l") {
			t.Fatalf("h/l round trip was not handled from %+v", want)
		}
		got := snapshotSelection(h.e, table)
		t.Logf("path A h/l round trip want=%+v got=%+v", want, got)
		if got != want {
			t.Fatalf("h/l round trip changed selection snapshot\nbefore: %+v\nafter:  %+v", want, got)
		}
	}
}

func TestVisualMotionOutwardFromBottomRightTableSelectionDoesNotWrap(t *testing.T) {
	const src = "| Name | Details | Count |\n| --- | --- | --- |\n| first | body | 17 |\n| last | bottom-right cell content | 2048 |\n\nafter table paragraph"
	for _, tc := range []struct {
		name string
		key  string
	}{
		{name: "horizontal-right", key: "l"},
		{name: "vertical-down", key: "j"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, src)
			h.e.layoutIfNeeded()
			var table *leaf
			for _, l := range h.e.leaves {
				if l.kind == kTable {
					table = l
					break
				}
			}
			if table == nil || len(table.tab.rows) < 3 {
				t.Fatal("bottom-right motion fixture did not parse")
			}
			h.e.SetKeymap("vim")
			h.e.vimController().SetMode(vim.ModeVisual)
			row := len(table.tab.rows) - 1
			cell := table.tab.rows[row][len(table.tab.rows[row])-1]
			h.e.setSessionAnchor(cell.end - 2)
			h.e.setSessionHead(cell.end)
			initial := snapshotSelection(h.e, table)
			t.Logf("path B initial key=%q state=%+v", tc.key, initial)
			h.shot("table-bottom-right-" + tc.name + "-before")
			for i := 1; i <= 3; i++ {
				if !h.e.modalText(tc.key) {
					t.Fatalf("outward %q motion %d was not handled; initial=%+v", tc.key, i, initial)
				}
				after := snapshotSelection(h.e, table)
				t.Logf("path B key=%q[%d] after=%+v", tc.key, i, after)
				h.shot(fmt.Sprintf("table-bottom-right-%s-after-%02d", tc.name, i))
				if after.caretRow == 0 && after.caretCol == 0 {
					t.Fatalf("outward %q motion wrapped to table's first row and first column; initial=%+v after=%+v", tc.key, initial, after)
				}
			}
		})
	}
}

// Partial fourth-row text remains the source selection while structure owns
// its range, and every step back must restore the complete semantic state.
func TestFourthRowPartialSelectionRoundTrip(t *testing.T) {
	h := newHarness(t, "| header | value |\n| --- | --- |\n| first | one |\n| second | two |\n| third | three |\n| fourth-long-text | four |")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	h.e.vimController().SetMode(vim.ModeVisual)
	cell := table.tab.rows[4][0]
	h.e.setSessionSelection(cell.start+9, cell.start+5)
	initialAnchor := h.e.Session.Selection().Anchor
	h.shot("fourth-partial-initial")
	states := []reversibleSelectionSnapshot{snapshotSelection(h.e, table)}
	for i := 0; i < 100; i++ {
		h.e.modalText("h")
		s := snapshotSelection(h.e, table)
		assertSelectionVariant(t, h.e)
		t.Logf("h %d: %+v", i+1, s)
		states = append(states, s)
		if s.hasTableSelection && s.tableActive == 3 {
			break
		}
	}
	if h.e.tableSelection == nil || h.e.tableSelection.active != 3 {
		t.Fatal("did not reach third row structure")
	}
	h.shot("fourth-partial-structure")
	if h.e.Session.Selection().Anchor != initialAnchor {
		t.Errorf("structure overwrote partial text anchor: got %d want %d", h.e.Session.Selection().Anchor, initialAnchor)
	}
	for i := len(states) - 2; i >= 0; i-- {
		h.e.modalText("l")
		got := snapshotSelection(h.e, table)
		assertSelectionVariant(t, h.e)
		t.Logf("l -> %d: %+v", i, got)
		if got != states[i] {
			t.Fatalf("fourth-row text expanded or path changed at %d\nwant %+v\ngot  %+v", i, states[i], got)
		}
	}
	h.shot("fourth-partial-restored")
}

func TestThreeRowStructureRetracesEveryStep(t *testing.T) {
	for _, tc := range []struct {
		name, key, back string
		start, target   int
		right           bool
	}{
		{"left", "h", "l", 3, 1, false}, {"right", "l", "h", 1, 3, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, "| header | value |\n| --- | --- |\n| first | one |\n| second | two |\n| third | three |\n| fourth | four |")
			h.e.SetKeymap("vim")
			h.e.layoutIfNeeded()
			var table *leaf
			for _, l := range h.e.leaves {
				if l.kind == kTable {
					table = l
					break
				}
			}
			h.e.vimController().SetMode(vim.ModeVisual)
			h.e.selectTableAxis(table, tableRows, tc.start, tc.start, false)
			h.e.tableSelection.right = tc.right
			states := []reversibleSelectionSnapshot{snapshotSelection(h.e, table)}
			for i := 0; i < 100; i++ {
				h.e.modalText(tc.key)
				s := snapshotSelection(h.e, table)
				states = append(states, s)
				assertSelectionVariant(t, h.e)
				t.Logf("%s %d: %+v", tc.key, i+1, s)
				if s.hasTableSelection && s.tableActive == tc.target {
					break
				}
			}
			if h.e.tableSelection == nil || h.e.tableSelection.first != 1 || h.e.tableSelection.last != 3 {
				t.Fatal("did not expand to three rows")
			}
			h.shot("three-row-" + tc.name + "-expanded")
			for i := len(states) - 2; i >= 0; i-- {
				h.e.modalText(tc.back)
				got := snapshotSelection(h.e, table)
				t.Logf("%s -> %d: %+v", tc.back, i, got)
				if got != states[i] {
					t.Fatalf("reverse step %d failed\nwant %+v\ngot  %+v", i, states[i], got)
				}
			}
			h.shot("three-row-" + tc.name + "-restored")
		})
	}
}

func TestTableSelectionOwnsRangeAndRestoresOnUndo(t *testing.T) {
	h := newHarness(t, "| a | b |\n| --- | --- |\n| one | two |\n| three | four |\n\nafter")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	h.e.vimController().SetMode(vim.ModeVisual)
	// Deliberately unrelated text endpoints cannot determine structure range/focus.
	h.e.setSessionSelection(len(h.e.Source())-3, len(h.e.Source())-1)
	textAnchor, textHead := h.e.Session.Selection().Anchor, h.e.Caret()
	h.e.selectTableAxis(table, tableRows, 1, 1, false)
	before := snapshotSelection(h.e, table)
	if h.e.Session.Selection().Anchor != textAnchor || h.e.Caret() != textHead {
		t.Fatal("structure changed dormant text")
	}
	assertSelectionVariant(t, h.e)
	wantStart, wantEnd, _ := h.e.tableSelectionSourceRange(table, h.e.tableSelection)
	if got := h.e.SelectionRange(); got.Start != wantStart || got.End != wantEnd {
		t.Fatalf("range follows dormant text: %+v", got)
	}
	if !h.e.applyTableSelection("d") || !h.e.Undo() {
		t.Fatal("delete/undo failed")
	}
	h.e.layoutIfNeeded()
	table = h.e.tableSelectionLeaf()
	if table == nil {
		t.Fatal("undo lost structure")
	}
	if got := snapshotSelection(h.e, table); got != before {
		t.Fatalf("undo lost active variant\nwant %+v\ngot %+v", before, got)
	}
	if !h.e.Redo() || h.e.tableSelection != nil {
		t.Fatal("redo retained stale structure")
	}
	h.e.Undo()
	h.e.layoutIfNeeded()
	table = h.e.tableSelectionLeaf()
	focus := h.e.selectionFocus()
	h.e.modalKey(ui.KeyEscape, 0)
	if h.e.Session.Selection().Anchor != focus || h.e.Caret() != focus || h.e.tableSelection != nil || h.e.tableController.NavigationLen() != 0 {
		t.Fatal("Esc did not collapse at structural focus")
	}
	h.e.selectTableAxis(table, tableRows, 1, 1, false)
	h.e.SelectAll()
	if h.e.tableSelection != nil || h.e.tableBridge != nil || h.e.SelectedText() != h.e.Source() {
		t.Fatal("SelectAll retained structural variant")
	}
}
