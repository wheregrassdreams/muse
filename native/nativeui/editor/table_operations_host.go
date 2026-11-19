package editor

import (
	"unicode/utf8"

	tablemodel "muse/editor/blocks/table"
	"muse/editor/input/vim"
	tableui "muse/nativeui/editor/blocks/table"
)

func (e *Editor) tableRegisterForSelection(t *leaf, s *tableSelection) *tablemodel.Register {
	if t == nil || s == nil || s.lineStart != t.lineStart {
		return nil
	}
	selection := tableSelectionSnapshot(s)
	if selection == nil {
		return nil
	}
	return tableui.RegisterForSelection(documentViewForTable(e, t), *selection)
}

// applyTableSelection performs a row or column edit in one undo step. The
// parser remains the authority for whether the result is still a GFM table.
func (e *Editor) applyTableSelection(op string) bool {
	s := e.tableSelection
	t := e.tableSelectionLeaf()
	if s == nil || t == nil {
		return false
	}
	selection := tableSelectionSnapshot(s)
	if selection == nil {
		return false
	}
	plan := tableui.PlanSelectionOperation(*selection, tablemodel.Grid{
		LineStart: t.lineStart, Rows: len(t.tab.rows), Cols: t.tab.cols(),
	}, op)
	if plan.ClearCellArea {
		e.setVimTableRegister(e.tableRegisterForSelection(t, s))
		if !e.clearTableCellRegion(t, *selection) {
			return false
		}
		if op == "c" {
			e.modalSetMode(vim.ModeInsert)
		} else {
			e.modalSetMode(vim.ModeNormal)
		}
		e.tableSelection = nil
		e.tableBridge = nil
		return true
	}
	if plan.Selection != *selection {
		e.tableSelection = tableSelectionFromSnapshot(&plan.Selection)
		s = e.tableSelection
	}
	reg := e.tableRegisterForSelection(t, s)
	e.setVimTableRegister(reg)
	if op == "y" {
		focus := e.selectionFocus()
		ranges := e.SelectionRanges()
		e.Copy()
		e.startVimYankFlash(ranges)
		// A yank doesn't leave a pending structural destination: the source
		// caret is still meaningful and p/P should resolve at its current cell.
		e.clearTableVirtualEdit()
		e.tableSelection = nil
		e.tableHandleDrag = nil
		e.tableBridge = nil
		e.resetTableNavigation()
		e.setSessionSelection(focus, focus)
		e.vimController().SetLinewise(false)
		e.setSessionInclusive(false)
		e.setSessionLinewise(false)
		if op == "d" {
			e.tableSelection, e.tableBridge, e.tableDocumentRegion = nil, nil, nil
			e.resetTableNavigation()
		}
		e.Session.UpdateLastHistoryAfter()
		return true
	}
	lines := e.tableLines(t)
	if len(lines) < 2 {
		return false
	}
	if s.axis == tableRows || s.axis == tableWhole {
		lineIndex := func(row int) int {
			if row == 0 {
				return 0
			}
			return row + 1
		}
		first, last := lineIndex(s.first), lineIndex(s.last)
		if s.axis == tableWhole {
			first, last = 0, len(lines)-1
		}
		if first >= len(lines) || last >= len(lines) {
			return false
		}
		virtualLineStart := t.lineStart
		virtualRow := s.first
		keepVirtual := op == "d" && s.axis == tableRows
		if s.axis == tableWhole {
			e.clearTableVirtualEdit()
		}
		result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.DeleteRows, LineStart: s.lineStart, First: s.first, Last: s.last, Whole: s.axis == tableWhole, Change: op == "c"})
		if err != nil || !result.Changed {
			return false
		}
		if keepVirtual {
			var updated *leaf
			for _, candidate := range e.leaves {
				if candidate.kind == kTable && candidate.tab != nil && candidate.lineStart == virtualLineStart {
					updated = candidate
					break
				}
			}
			if updated != nil && len(updated.tab.rows) > 0 {
				virtualRow = max(0, min(virtualRow, len(updated.tab.rows)-1))
				e.enterTableVirtualEdit(updated, virtualRow, 0, tableVirtualLeading, tableRows, true)
			} else {
				e.clearTableVirtualEdit()
			}
		}
		if op == "c" {
			e.modalSetMode(vim.ModeInsert)
		}
		e.vimController().SetLinewise(false)
		e.setSessionInclusive(false)
		e.setSessionLinewise(false)
		if op == "d" {
			e.tableSelection, e.tableBridge, e.tableDocumentRegion = nil, nil, nil
			e.resetTableNavigation()
		}
		e.Session.UpdateLastHistoryAfter()
		return true
	}
	if s.axis == tableColumns {
		virtualLineStart := t.lineStart
		virtualRow := max(0, min(s.activeRow, len(t.tab.rows)-1))
		virtualCol := s.first
		keepVirtual := op == "d"
		result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.DeleteColumns, LineStart: s.lineStart, First: s.first, Last: s.last, Change: op == "c"})
		if err != nil || !result.Changed {
			return false
		}
		if keepVirtual {
			var updated *leaf
			for _, candidate := range e.leaves {
				if candidate.kind == kTable && candidate.tab != nil && candidate.lineStart == virtualLineStart {
					updated = candidate
					break
				}
			}
			if updated != nil && updated.tab.cols() > 0 && len(updated.tab.rows) > 0 {
				virtualRow = max(0, min(virtualRow, len(updated.tab.rows)-1))
				virtualCol = max(0, min(virtualCol, updated.tab.cols()-1))
				e.enterTableVirtualEdit(updated, virtualRow, virtualCol, tableVirtualLeading, tableColumns, true)
			} else {
				e.clearTableVirtualEdit()
			}
		}
		if op == "c" {
			e.modalSetMode(vim.ModeInsert)
			e.modal.blockInsert = &tableBlockInsert{lineStart: s.lineStart, firstCol: s.first}
		}
		e.vimController().SetLinewise(false)
		e.setSessionInclusive(false)
		e.setSessionLinewise(false)
		return true
	}
	return false
}

func (e *Editor) typeTableColumnSelection(text string) bool {
	s := e.tableSelection
	t := e.tableSelectionLeaf()
	if s == nil || s.axis != tableColumns || t == nil {
		return false
	}
	result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.TypeColumnSelection, LineStart: s.lineStart, First: s.first, Last: s.last, Text: text})
	if err != nil || !result.Changed {
		return false
	}
	e.modalSetMode(vim.ModeInsert)
	e.modal.blockInsert = &tableBlockInsert{lineStart: s.lineStart, firstCol: s.first, text: text}
	return true
}

func (e *Editor) typeTableBlock(text string) bool {
	b := e.modal.blockInsert
	if b == nil {
		return false
	}
	result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.TypeBlock, LineStart: b.lineStart, Focus: tablemodel.Cell{Col: b.firstCol}, Text: text})
	if err != nil || !result.Changed {
		e.modal.blockInsert = nil
		return false
	}
	b.text += text
	return true
}

func (e *Editor) backspaceTableBlock() bool {
	b := e.modal.blockInsert
	if b == nil || b.text == "" {
		return b != nil
	}
	_, n := utf8.DecodeLastRuneInString(b.text)
	remove := b.text[len(b.text)-n:]
	result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.BackspaceBlock, LineStart: b.lineStart, Focus: tablemodel.Cell{Col: b.firstCol}, Text: remove})
	if err != nil || !result.Changed {
		e.modal.blockInsert = nil
		return false
	}
	b.text = b.text[:len(b.text)-n]
	return true
}

func (e *Editor) pasteTableRegister(before bool) bool {
	r := e.modal.registerTable
	if r == nil || len(r.Rows()) == 0 {
		return false
	}
	if s := e.tableSelection; s != nil && s.cellRegion {
		if t := e.tableSelectionLeaf(); t != nil {
			region, ok := e.tableSelectionCellRegion(s)
			matrix := &tableCellMatrix{Cells: r.RowsWithHeader(), Alignments: r.Alignments()}
			if ok && e.replaceTableCellRegion(t, region, matrix) {
				e.tableSelection = nil
				e.tableBridge = nil
				e.tableDocumentRegion = nil
				e.Session.UpdateLastHistoryAfter()
				return true
			}
		}
	}
	// A structural delete/yank leaves its target as row/column coordinates.
	// Resolve that semantic position first: the text caret may be on a pipe or
	// separator where unitAt has no cell to return.
	var t *leaf
	row, col := -1, -1
	virtualLineStart := -1
	if r.Region() && r.Axis() == tableCells {
		t, row, col, _ = e.tableRegionAtCaret()
	}
	if t != nil {
		// Region registers use the focused cell as their semantic origin.
	} else if virtualTable, position, ok := e.resolveTableVirtualEdit(); ok {
		t, row, col = virtualTable, position.row, position.col
		virtualLineStart = position.lineStart
	} else {
		e.clearTableVirtualEdit()
		u := e.unitAt(e.Caret())
		if u != nil {
			t = e.tableOf(u)
			if t != nil {
				_, row, col = cellAt(t, e.Caret())
			}
		}
		if t == nil {
			if edge, after := e.tableAtEdge(e.Caret()); edge != nil {
				t = edge
				row, col = 0, 0
				if after && len(t.tab.rows) > 0 {
					row = len(t.tab.rows) - 1
					col = max(0, len(t.tab.rows[row])-1)
				}
			}
		}
		if t == nil {
			for _, candidate := range e.leaves {
				if candidate.kind != kTable || candidate.tab == nil || e.Caret() < candidate.lineStart || e.Caret() > blockEnd(e.Source(), candidate) {
					continue
				}
				t = candidate
				row, col, _ = e.tablePositionAtSource(t, e.Caret())
				break
			}
		} else if row < 0 || col < 0 {
			row, col, _ = e.tablePositionAtSource(t, e.Caret())
		}
	}
	if t != nil && virtualLineStart < 0 {
		virtualLineStart = t.lineStart
	}
	if t == nil || t.tab == nil || r.Whole() {
		value := r.Markdown()
		if value == "" {
			return false
		}
		at, end := e.modalLine(e.selectionFocus())
		if !before {
			at = end
		}
		if t != nil {
			at = t.lineStart
			if !before {
				at = blockEnd(e.Source(), t)
			}
		}
		result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.PasteBlock, First: at, Text: value})
		if err != nil || !result.Changed {
			return false
		}
		e.clearTableVirtualEdit()
		return true
	}
	lines := e.tableLines(t)
	if len(lines) < 2 || row < 0 || col < 0 {
		return false
	}
	if r.Region() && r.Axis() == tableCells {
		startCol := col
		if !before {
			startCol++
		}
		region := tablemodel.Selection{
			LineStart: t.lineStart, Anchor: tablemodel.Cell{Row: row, Col: startCol},
			Focus: tablemodel.Cell{Row: row, Col: startCol}, CellRegion: true,
		}
		if e.replaceTableCellRegion(t, region, &tableCellMatrix{Cells: r.Rows(), Alignments: r.Alignments()}) {
			e.clearTableVirtualEdit()
			return true
		}
		return false
	}
	if r.Axis() == tableRows {
		result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.PasteRows, LineStart: t.lineStart, First: row, Before: before, Matrix: tablemodel.Matrix{Cells: r.Rows()}})
		if err != nil || !result.Changed {
			return false
		}
		insertedRow := result.Insertion
		var updated *leaf
		for _, candidate := range e.leaves {
			if candidate.kind == kTable && candidate.tab != nil && candidate.lineStart == t.lineStart {
				updated = candidate
				break
			}
		}
		if updated != nil && len(updated.tab.rows) > 0 {
			insertedRow = max(0, min(insertedRow, len(updated.tab.rows)-1))
			e.enterTableVirtualEdit(updated, insertedRow, 0, tableVirtualLeading, tableRows, false)
		} else {
			e.clearTableVirtualEdit()
		}
		e.tableSelection, e.tableBridge, e.tableDocumentRegion = nil, nil, nil
		e.Session.UpdateLastHistoryAfter()
		return true
	}
	if r.Axis() == tableColumns {
		virtualRow := row
		col = max(0, min(col, t.tab.cols()-1))
		if !before {
			col++
		}
		matrix := r.Matrix()
		result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.PasteColumns, LineStart: virtualLineStart, First: virtualRow, Focus: tablemodel.Cell{Col: col}, Matrix: matrix})
		if err != nil || !result.Changed {
			return false
		}
		var updated *leaf
		for _, candidate := range e.leaves {
			if candidate.kind == kTable && candidate.tab != nil && candidate.lineStart == virtualLineStart {
				updated = candidate
				break
			}
		}
		if updated != nil && len(updated.tab.rows) > 0 {
			virtualRow = max(0, min(virtualRow, len(updated.tab.rows)-1))
			insertedCol := max(0, min(col, updated.tab.cols()-1))
			if insertedCol < len(updated.tab.rows[virtualRow]) {
				e.enterTableVirtualEdit(updated, virtualRow, insertedCol, tableVirtualLeading, tableColumns, false)
			} else {
				e.clearTableVirtualEdit()
			}
		} else {
			e.clearTableVirtualEdit()
		}
		e.tableSelection, e.tableBridge, e.tableDocumentRegion = nil, nil, nil
		e.Session.UpdateLastHistoryAfter()
		return true
	}
	return false
}
