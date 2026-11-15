package mdedit

// tableRegionAtCaret returns parser-owned cell coordinates for the current
// caret. It is shared by Vim entry and directional region extension.
func (e *Editor) tableRegionAtCaret() (*leaf, int, int, bool) {
	return e.tableCellAtSourceOffset(e.caret)
}

func (e *Editor) tableCellAtSourceOffset(off int) (*leaf, int, int, bool) {
	for _, t := range e.leaves {
		if t.kind != kTable || t.tab == nil || off < t.lineStart || off > blockEnd(e.src, t) {
			continue
		}
		for row, cells := range t.tab.rows {
			for col, cell := range cells {
				if off >= cell.start && off <= cell.end {
					return t, row, col, true
				}
			}
		}
		row, col, ok := e.tablePositionAtSource(t, off)
		return t, row, col, ok
	}
	return nil, 0, 0, false
}

// promoteVisualTextToCellRegion upgrades Vim v after its active text range
// reaches a distinct cell in the same table. Once promoted, tableSelection is
// the sole active selection and repeated motion back to the anchor stays in
// structural mode.
func (e *Editor) promoteVisualTextToCellRegion() bool {
	if !e.modal.vimTextVisualActive || e.tableSelection != nil && e.tableSelection.cellRegion || e.tableBridge != nil || e.modal.mode != "Visual" || e.modal.line || e.selectionLinewise || e.Keymap() != "vim" {
		return false
	}
	anchorTable, anchorRow, anchorCol, anchorOK := e.tableCellAtSourceOffset(e.anchor)
	focusTable, focusRow, focusCol, focusOK := e.tableCellAtSourceOffset(e.caret)
	if !anchorOK || !focusOK || anchorTable.lineStart != focusTable.lineStart || anchorRow == focusRow && anchorCol == focusCol {
		return false
	}
	if e.selectTableCellRegion(anchorTable, anchorRow, anchorCol, focusRow, focusCol) {
		// Lowercase Vim Visual starts as a text range, then promotes to cells.
		// Keep its source-row traversal identity so h/l can continue across the
		// row boundary into the adjacent table row instead of dropping the
		// header row or leaving a stale text caret behind.
		e.tableSelection.fromText = true
		e.tableSelection.textMode = "Visual"
		e.modal.vimTextVisualActive = false
		return true
	}
	return false
}

// moveTableCellRegion extends the active rectangle by one cell. Returning
// false leaves boundary handling to the existing document navigation path.
func (e *Editor) moveTableCellRegion(rowDelta, colDelta int) bool {
	before := e.tableNavigationState()
	if e.replayTableNavigation(rowDelta, colDelta) {
		return true
	}
	s := e.tableSelection
	if s == nil || !s.cellRegion {
		return false
	}
	e.tableNavigating = true
	defer func() { e.tableNavigating = false }()
	t := e.tableByLineStart(s.lineStart)
	if t == nil {
		return false
	}
	row, col := s.focusRow+rowDelta, s.focusCol+colDelta
	if rowDelta == 0 && s.fromText && s.textMode == "Visual" && e.modal.mode == "Visual" {
		switch {
		case colDelta < 0 && s.focusCol == 0 && s.focusRow > 0:
			// Vim Visual h continues the row selection at the same column in
			// the preceding rendered row. This lets a horizontal sweep from
			// the first body row include its table header as one rectangle.
			row--
			col = min(s.focusCol, len(t.tab.rows[row])-1)
		case colDelta > 0 && s.focusRow+1 < len(t.tab.rows) && s.focusCol == len(t.tab.rows[s.focusRow])-1:
			// l is the inverse row transition, keeping the same column.
			row++
			col = min(s.focusCol, len(t.tab.rows[row])-1)
		}
	}
	if row < 0 || row >= len(t.tab.rows) || col < 0 || col >= t.tab.cols() {
		anchorCell := t.tab.rows[max(0, min(s.anchorRow, len(t.tab.rows)-1))][max(0, min(s.anchorCol, len(t.tab.rows[max(0, min(s.anchorRow, len(t.tab.rows)-1))])-1))]
		focusRow := max(0, min(s.focusRow, len(t.tab.rows)-1))
		focusCol := max(0, min(s.focusCol, len(t.tab.rows[focusRow])-1))
		focusCell := t.tab.rows[focusRow][focusCol]
		anchorOffset, focusOffset := anchorCell.start, focusCell.start
		if rowDelta < 0 || colDelta < 0 {
			focusOffset = focusCell.start
		} else {
			focusOffset = focusCell.end
		}
		preserved := *s
		e.tableDocumentRegion = &preserved
		e.clearTableSelection()
		e.anchor, e.caret = anchorOffset, focusOffset
		e.selectionInclusive = false
		e.selectionLinewise = false
		e.leaveTableBoundary(t, firstNonzero(rowDelta, colDelta), true)
		e.recordTableNavigation(before, rowDelta, colDelta)
		return true
	}
	fromText, textMode := s.fromText, s.textMode
	if !e.selectTableCellRegion(t, s.anchorRow, s.anchorCol, row, col) {
		return false
	}
	e.tableSelection.fromText = fromText
	e.tableSelection.textMode = textMode
	e.recordTableNavigation(before, rowDelta, colDelta)
	return true
}

func firstNonzero(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}
