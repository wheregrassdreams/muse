package editor

import "muse/editor/input/vim"

func (e *Editor) startTableVisualLine(focus int) bool {
	e.layoutIfNeeded()
	cell := e.unitAt(focus)
	if cell == nil || cell.table == nil {
		return false
	}
	t := cell.table
	_, row, _ := cellAt(t, focus)
	if row < 0 || row >= len(t.tab.rows) {
		return false
	}
	e.block = nil
	e.blockInput = nil
	e.selectTableAxis(t, tableRows, row, row, false)
	e.setSessionSelection(focus, focus)
	e.setSessionInclusive(false)
	e.setSessionLinewise(false)
	e.finishNativeLineSelection()
	return true
}

func (e *Editor) tableVisualLineActive() bool {
	return e.vimController().Mode() == vim.ModeVisual && e.vimController().View().Linewise && e.tableSelection != nil && e.tableSelection.axis == tableRows
}

func (e *Editor) tableVisualLineMode() bool {
	return e.vimController().Mode() == vim.ModeVisual && e.vimController().View().Linewise
}

func (e *Editor) moveTableVisualLineHorizontal(dir int) {
	if e.tableSelection == nil || dir == 0 {
		return
	}
	e.tableSelection.right = dir > 0
	e.hasGoal = false
	e.upstream = false
	e.wantReveal = true
	e.invalidate()
}

func (e *Editor) tableVisualCaretAtRow(t *leaf, row, off int) int {
	if t == nil || row < 0 || row >= len(t.tab.rows) || len(t.tab.rows[row]) == 0 {
		return off
	}
	col, relative := 0, 0
	if cell := e.unitAt(off); cell != nil && cell.table == t {
		if sourceCell, oldRow, oldCol := cellAt(t, off); sourceCell != nil && oldRow >= 0 && oldCol >= 0 {
			col = min(oldCol, len(t.tab.rows[row])-1)
			relative = max(0, off-sourceCell.start)
		}
	}
	target := t.tab.rows[row][col]
	return min(target.end, target.start+relative)
}

// moveTableVisualLineCaret applies a source/word motion to the selected row:
// targets inside the table select their rendered row, and targets outside turn
// the structure into a mixed linewise document selection.
func (e *Editor) moveTableVisualLineCaret(off int) bool {
	s := e.tableSelection
	t := e.tableSelectionLeaf()
	if s == nil {
		return e.restoreTableVisualLineTarget(off)
	}
	if t == nil || s.axis != tableRows {
		return false
	}
	row := e.tableVisualRowAtOffset(t, off)
	if row >= 0 && row < len(t.tab.rows) {
		right := s.right
		e.selectTableAxis(t, tableRows, s.anchor, row, false)
		e.tableSelection.right = right
		e.setSessionHead(off)
		e.setSessionInclusive(false)
		e.setSessionLinewise(false)
		e.upstream = false
		e.wantReveal = true
		e.invalidate()
		return true
	}
	anchorRow := s.anchor
	e.tableSelection = nil
	e.tableBridge = nil
	e.setSessionInclusive(false)
	e.setSessionLinewise(true)
	e.setSessionAnchor(e.tableRowBoundary(t, anchorRow, false))
	e.setSessionHead(max(0, min(off, len(e.Source()))))
	e.hasGoal = false
	e.upstream = false
	e.wantReveal = true
	e.invalidate()
	return true
}

func (e *Editor) restoreTableVisualLineTarget(off int) bool {
	if !e.tableVisualLineMode() || !e.Session.Selection().Linewise || e.tableSelection != nil {
		return false
	}
	a, z := e.selection()
	for _, t := range e.leaves {
		if t.kind != kTable || t.tab == nil {
			continue
		}
		row := e.tableVisualRowAtOffset(t, off)
		if row < 0 {
			continue
		}
		mixed := e.tableStructureInRange(t, a, z)
		if mixed == nil || mixed.axis != tableRows && mixed.axis != tableWhole {
			continue
		}
		anchor := mixed.anchor
		if mixed.axis == tableWhole {
			if mixed.right {
				anchor = 0
			} else {
				anchor = len(t.tab.rows) - 1
			}
		}
		right := mixed.right
		e.selectTableAxis(t, tableRows, anchor, row, false)
		e.tableSelection.right = right
		e.setSessionHead(off)
		e.setSessionLinewise(false)
		e.setSessionInclusive(false)
		e.wantReveal = true
		e.invalidate()
		return true
	}
	return false
}

func (e *Editor) tableVisualRowAtOffset(t *leaf, off int) int {
	if hit, row, _, ok := e.tableBoundaryAtOffset(off); ok && hit == t {
		return row
	}
	if cell := e.unitAt(off); cell != nil && cell.table == t {
		_, row, _ := cellAt(t, off)
		return row
	}
	if t == nil || t.tab == nil || off < t.lineStart || off > blockEnd(e.Source(), t) {
		return -1
	}
	lineStart := lineStartAt(e.Source(), off)
	for line, span := range e.tableLines(t) {
		if lineStart < span[0] || lineStart > span[1] {
			continue
		}
		switch {
		case line == 0 || line == 1:
			return 0
		default:
			return line - 1
		}
	}
	return -1
}

// restoreTableVisualLineFromMixed brings a linewise selection back into its
// structural table rows when j/k reverses a one-row exit from that table.
func (e *Editor) restoreTableVisualLineFromMixed(dir int) bool {
	if e.vimController().Mode() != vim.ModeVisual || !e.vimController().View().Linewise || !e.Session.Selection().Linewise || e.tableSelection != nil || dir == 0 {
		return false
	}
	a, z := e.selection()
	for _, t := range e.leaves {
		if t.kind != kTable || t.tab == nil {
			continue
		}
		end := blockEnd(e.Source(), t)
		adjacent := dir > 0 && e.Caret() < t.lineStart && lineEndAt(e.Source(), e.Caret())+1 >= t.lineStart ||
			dir < 0 && e.Caret() > end && lineStartAt(e.Source(), e.Caret())-1 <= end
		if !adjacent {
			continue
		}
		s := e.tableStructureInRange(t, a, z)
		if s == nil || s.axis != tableRows && s.axis != tableWhole {
			continue
		}
		if s.axis == tableWhole {
			s.axis = tableRows
			s.first, s.last = 0, len(t.tab.rows)-1
			if s.right {
				s.anchor, s.active = s.first, s.last
			} else {
				s.anchor, s.active = s.last, s.first
			}
			s.activeRow = s.active
		}
		e.tableSelection = s
		e.tableBridge = nil
		e.setSessionLinewise(false)
		e.setSessionInclusive(false)
		e.hasGoal = false
		e.wantReveal = true
		e.invalidate()
		return true
	}
	return false
}

// nativeStartTableColumnVisual enters the table's structural column mode at
// the current cell. It lets Ctrl+V keep the table-specific column contract.
func (e *Editor) nativeStartTableColumnVisual(focus int) bool {
	e.layoutIfNeeded()
	cell := e.unitAt(focus)
	if cell == nil || cell.table == nil {
		return false
	}
	t := cell.table
	_, row, col := cellAt(t, focus)
	if row < 0 || col < 0 {
		return false
	}
	e.block = nil
	e.blockInput = nil
	e.selectTableAxis(t, tableColumns, col, col, false)
	e.tableSelection.activeRow = row
	e.setSessionInclusive(false)
	e.setSessionLinewise(false)
	e.finishNativeLineSelection()
	return true
}

// moveTableVisualColumn moves the active focus by rendered table row while
// preserving the selected column. Leaving the table collapses the structural
// selection and transfers focus to the adjacent visible document leaf.
func (e *Editor) moveTableVisualColumn(dir, count int) bool {
	if e.vimController().Mode() != vim.ModeBlockVisual || e.tableSelection == nil || e.tableSelection.axis != tableColumns || dir == 0 {
		return false
	}
	if count < 1 {
		count = 1
	}
	t := e.tableSelectionLeaf()
	if t == nil || len(t.tab.rows) == 0 {
		return false
	}
	for range count {
		next := e.tableSelection.activeRow + dir
		if next < 0 || next >= len(t.tab.rows) {
			focus := e.selectionFocus()
			e.tableSelection = nil
			e.tableBridge = nil
			e.resetTableNavigation()
			e.block = nil
			e.modalSetMode(vim.ModeNormal)
			e.vimController().SetLinewise(false)
			e.setSessionInclusive(false)
			e.setSessionLinewise(false)
			e.setSessionSelection(focus, focus)
			e.leaveTableBoundary(t, dir, false)
			return true
		}
		e.tableSelection.activeRow = next
		e.hasGoal = false
		e.wantReveal = true
		e.invalidate()
	}
	return true
}
