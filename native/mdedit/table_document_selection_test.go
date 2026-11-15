package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

const documentTable = "before\n\n| 左对齐 | 居中 | 右对齐 |\n| :--- | :---: | ---: |\n| 甲 | `code` | 42 |\n| 双链 | $x^2$ | 注释 |\n\nafter"

func documentTestTable(t *testing.T, e *Editor) *leaf {
	t.Helper()
	e.layoutIfNeeded()
	for _, l := range e.leaves {
		if l.kind == kTable {
			return l
		}
	}
	t.Fatalf("no table in %q", e.Source())
	return nil
}

func TestCopyAllTableRowsPastesCompleteTable(t *testing.T) {
	for _, axis := range []tableAxis{tableRows, tableWhole, tableColumns} {
		t.Run(string(rune('0'+axis)), func(t *testing.T) {
			h := newHarness(t, documentTable)
			table := documentTestTable(t, h.e)
			last := len(table.tab.rows) - 1
			if axis == tableColumns {
				last = table.tab.cols() - 1
			}
			h.e.selectTableAxis(table, axis, 0, last, false)
			if !h.e.Copy() {
				t.Fatal("copy failed")
			}
			md, _ := h.clip.Read(transfer.Markdown)
			html, _ := h.clip.Read(transfer.HTML)
			if !strings.Contains(string(html), "<table>") {
				t.Fatalf("clipboard is not a table: %q", md)
			}
			dst := newHarness(t, "")
			dst.clip = h.clip
			dst.e.Paste(false)
			pasted := documentTestTable(t, dst.e)
			dst.shot("pasted-complete-table")
			if len(pasted.tab.rows) != len(table.tab.rows) || pasted.tab.cols() != table.tab.cols() {
				t.Fatalf("pasted dimensions differ: %q", dst.e.Source())
			}
			for c, align := range table.tab.align {
				if pasted.tab.align[c] != align {
					t.Fatalf("alignment lost in column %d: %q", c, dst.e.Source())
				}
			}
			// Replacing the same structural selection must not escape the
			// Markdown as if it were a single cell's text.
			h.e.Paste(false)
			if got := documentTestTable(t, h.e); len(got.tab.rows) != len(table.tab.rows) {
				t.Fatalf("structural paste replacement damaged table: %q", h.e.Source())
			}
		})
	}
}

func TestDocumentSelectionUsesTableStructure(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		h := newHarness(t, documentTable)
		table := documentTestTable(t, h.e)
		h.e.anchor, h.e.caret = 0, len(documentTable)
		if reverse {
			h.e.anchor, h.e.caret = h.e.caret, h.e.anchor
		}
		h.e.structuralSelectionForRange()
		h.frame()
		parts := h.e.selectionParts()
		if len(parts) != 3 || parts[0].structure != nil || parts[1].structure == nil || parts[1].structure.axis != tableWhole || parts[2].structure != nil {
			t.Fatalf("selection did not compose text/table/text: %+v", parts)
		}
		if got := h.e.SelectedText(); got != documentTable {
			t.Fatalf("mixed selection lost surrounding text: %q", got)
		}
		for _, row := range table.tab.rows {
			for _, cell := range row {
				if cell.view != nil && cell.view.params.selection != [2]int{} {
					t.Fatalf("table structure also received text selection: %+v", cell.view.params.selection)
				}
			}
		}
		h.shot("document-table-structure")
		if !h.e.Cut() || h.e.Source() != "" || !h.e.Undo() || h.e.Source() != documentTable {
			t.Fatal("mixed cut/undo did not preserve the document")
		}
	}
}

func TestDocumentVerticalSelectionCrossesTableAtomically(t *testing.T) {
	for _, dir := range []int{-1, 1} {
		h := newHarness(t, documentTable)
		table := documentTestTable(t, h.e)
		h.e.SetKeymap("vim")
		h.e.modal.mode = "Visual"
		start := 0
		key, back := "j", "k"
		if dir < 0 {
			start, key, back = len(documentTable)-len("after"), "k", "j"
		}
		h.e.anchor, h.e.caret = start, start
		moves, seen := 0, false
		for ; moves < 12; moves++ {
			h.e.modalText(key)
			focus := h.e.selectionFocus()
			t.Logf("%s %d: %s", key, moves+1, tableSelectionState(h.e))
			if focus >= table.lineStart && focus <= blockEnd(h.e.src, table) {
				seen = true
				if s := h.e.tableStructureSelection(table); s == nil || s.axis != tableWhole {
					t.Fatalf("vertical document motion entered cell content: %s", tableSelectionState(h.e))
				}
			}
			if seen && (dir > 0 && focus > blockEnd(h.e.src, table) || dir < 0 && focus < table.lineStart) {
				moves++
				break
			}
		}
		if !seen || moves >= 12 {
			t.Fatal("did not cross table")
		}
		for i := 0; i < moves; i++ {
			h.e.modalText(back)
		}
		if h.e.anchor != start || h.e.caret != start || h.e.tableStructureSelection(table) != nil {
			t.Fatalf("reverse motion did not shrink to original paragraph: %s", tableSelectionState(h.e))
		}
	}
}

func TestDocumentSelectionComposesMultipleTables(t *testing.T) {
	src := documentTable + "\n\n" + documentTable
	h := newHarness(t, src)
	h.e.SelectAll()
	h.frame()
	count := 0
	for _, part := range h.e.selectionParts() {
		if part.structure != nil {
			count++
			if part.structure.axis != tableWhole {
				t.Fatal("complete table became partial structure")
			}
		}
	}
	if count != 2 || h.e.SelectedText() != src {
		t.Fatalf("multiple-table selection count=%d text=%q", count, h.e.SelectedText())
	}
}

func TestWholeTableRegisterPastesAsSeparateTable(t *testing.T) {
	for _, inTable := range []bool{false, true} {
		for _, before := range []bool{false, true} {
			h := newHarness(t, documentTable)
			table := documentTestTable(t, h.e)
			h.e.modal.registerTable = h.e.tableRegisterForSelection(table, &tableSelection{lineStart: table.lineStart, axis: tableRows, first: 0, last: len(table.tab.rows) - 1})
			h.e.caret = len(documentTable)
			if inTable {
				h.e.caret = table.tab.rows[1][0].start
			}
			if !h.e.pasteTableRegister(before) {
				t.Fatal("register paste failed")
			}
			h.e.layoutIfNeeded()
			count := 0
			for _, l := range h.e.leaves {
				if l.kind == kTable {
					count++
					if len(l.tab.rows) != 3 {
						t.Fatalf("pasted table merged or lost rows: %q", h.e.Source())
					}
				}
			}
			if count != 2 {
				t.Fatalf("want two tables, got %d: %q", count, h.e.Source())
			}
		}
	}
}

func TestStructuralSelectionBoundaryCanLeaveTable(t *testing.T) {
	for _, dir := range []int{-1, 1} {
		for _, modal := range []bool{false, true} {
			h := newHarness(t, documentTable)
			table := documentTestTable(t, h.e)
			if modal {
				h.e.SetKeymap("vim")
				h.e.modal.mode = "Visual"
			}
			anchor, active := 0, len(table.tab.rows)-1
			if dir < 0 {
				anchor, active = active, anchor
			}
			h.e.selectTableAxis(table, tableRows, anchor, active, false)
			if modal {
				key := "j"
				if dir < 0 {
					key = "k"
				}
				h.e.modalText(key)
			} else {
				key := ui.KeyDown
				if dir < 0 {
					key = ui.KeyUp
				}
				h.key(ui.Shift, key)
			}
			if dir < 0 && h.e.selectionFocus() >= table.lineStart || dir > 0 && h.e.selectionFocus() <= blockEnd(h.e.src, table) {
				t.Fatalf("boundary stayed inside table dir=%d modal=%v: %s", dir, modal, tableSelectionState(h.e))
			}
			a, z := h.e.selection()
			if a > table.lineStart || z < blockEnd(h.e.src, table) {
				t.Fatalf("leaving lost the selected table: %s", tableSelectionState(h.e))
			}
		}
	}
}

func TestStructuralSelectionCanLeaveDocumentEdge(t *testing.T) {
	for _, dir := range []int{-1, 1} {
		src := "| a | b |\n| --- | --- |\n| 1 | 2 |"
		h := newHarness(t, src)
		table := documentTestTable(t, h.e)
		h.e.SetKeymap("vim")
		h.e.modal.mode = "Visual"
		first, last := 0, len(table.tab.rows)-1
		key, back := "j", "k"
		if dir < 0 {
			first, last, key, back = last, first, "k", "j"
		}
		h.e.selectTableAxis(table, tableRows, first, last, false)
		h.e.modalText(key)
		h.frame()
		table = documentTestTable(t, h.e)
		if h.e.phantom == nil || !h.e.leafIsPhantom(h.e.caret) || h.e.SelectedText() != src {
			t.Fatalf("table edge cannot enter outside empty paragraph: %s", tableSelectionState(h.e))
		}
		r, ok := h.e.caretRect(h.e.caret)
		if !ok || dir > 0 && r.Y < table.y+table.h || dir < 0 && r.Y >= table.y {
			t.Fatalf("caret did not render outside table: %+v table y=%f h=%f", r, table.y, table.h)
		}
		h.e.modalText(back)
		if h.e.phantom != nil || h.e.tableStructureSelection(documentTestTable(t, h.e)) == nil {
			t.Fatal("reverse did not reenter table structure")
		}
		h.e.modalText(key)
		h.e.insertText("replacement")
		if h.e.Source() != "replacement" || !h.e.Undo() || h.e.Source() != src {
			t.Fatalf("outside-head replacement/undo damaged range: %q", h.e.Source())
		}
	}
}
