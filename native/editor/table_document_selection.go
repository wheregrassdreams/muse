package editor

import tablemodel "muse/editor/blocks/table"

// selectionPart is a semantic unit in a document selection. Text before and
// after a table retains its precise source range; complete table structures
// own their range and highlighting, just like an explicitly selected row.
type selectionPart struct {
	source    Range
	structure *tableSelection
}

func cloneTableSelection(s *tableSelection) *tableSelection {
	if s == nil {
		return nil
	}
	copy := *s
	return &copy
}

func (e *Editor) selectionParts() []selectionPart {
	a, z := e.selection()
	if a == z {
		return nil
	}
	var parts []selectionPart
	start := a
	for _, t := range e.leaves {
		if t.kind != kTable || t.tab == nil {
			continue
		}
		s := e.tableStructureSelection(t)
		if s == nil {
			continue
		}
		// The semantic table segment owns the whole source block. Unselected
		// Markdown cells and delimiters inside that block must not leak out as
		// ordinary text parts around a row/column/cell projection.
		lo, hi := t.lineStart, blockEnd(e.src, t)
		if start < lo {
			parts = append(parts, selectionPart{source: Range{start, lo}})
		}
		parts = append(parts, selectionPart{source: Range{lo, hi}, structure: s})
		start = hi
	}
	if start < z {
		parts = append(parts, selectionPart{source: Range{start, z}})
	}
	return parts
}

func (e *Editor) tableStructureSelection(t *leaf) *tableSelection {
	if t == nil {
		return nil
	}
	if s := e.tableSelection; s != nil {
		if s.lineStart == t.lineStart {
			return s
		}
	}
	if s := e.tableDocumentRegion; s != nil && s.lineStart == t.lineStart {
		a, z := e.selection()
		if a < blockEnd(e.src, t) && z > t.lineStart {
			return s
		}
	}
	if e.block != nil {
		return nil
	}
	a, z := e.selection()
	return e.tableStructureInRange(t, a, z)
}

func (e *Editor) tableStructureInRange(t *leaf, a, z int) *tableSelection {
	if t == nil || t.tab == nil || len(t.tab.rows) == 0 {
		return nil
	}
	end := blockEnd(e.src, t)
	if a >= end || z <= t.lineStart {
		return nil
	}
	if a <= t.lineStart && z >= end {
		lastRow := len(t.tab.rows) - 1
		lastCol := max(0, len(t.tab.rows[lastRow])-1)
		if e.caret < e.anchor {
			return tableRegionSelection(t, lastRow, lastCol, 0, 0, false)
		}
		return tableRegionSelection(t, 0, 0, lastRow, lastCol, true)
	}
	// Ranges entirely inside the table remain text selections. A document range
	// crossing either boundary gets a rectangular structural segment for only
	// the table intersection (including the complete table case).
	if a >= t.lineStart && z <= end {
		return nil
	}
	lo, hi := max(a, t.lineStart), min(z, end)
	if lo >= hi {
		return nil
	}
	anchorRow, anchorCol := 0, 0
	if lo > t.lineStart {
		anchorRow, anchorCol = e.tableCellAtRangeBoundary(t, lo)
	}
	lastRow := len(t.tab.rows) - 1
	lastCol := max(0, len(t.tab.rows[lastRow])-1)
	focusRow, focusCol := lastRow, lastCol
	if hi < end {
		focusRow, focusCol = e.tableCellAtRangeBoundary(t, hi-1)
	}
	if e.caret < e.anchor {
		anchorRow, focusRow = focusRow, anchorRow
		anchorCol, focusCol = focusCol, anchorCol
	}
	return tableRegionSelection(t, anchorRow, anchorCol, focusRow, focusCol, e.caret >= e.anchor)
}

// tableCellAtRangeBoundary maps source offsets through parser-owned cell spans.
// Pipe and separator characters fall back to the projection's line mapping.
func (e *Editor) tableCellAtRangeBoundary(t *leaf, off int) (int, int) {
	if _, row, col := cellAt(t, off); row >= 0 && col >= 0 {
		return row, col
	}
	row, col, ok := e.tablePositionAtSource(t, off)
	if !ok {
		return 0, 0
	}
	return row, col
}

func tableRegionSelection(t *leaf, anchorRow, anchorCol, focusRow, focusCol int, right bool) *tableSelection {
	if t == nil || t.tab == nil {
		return nil
	}
	selection := (tablemodel.Grid{LineStart: t.lineStart, Rows: len(t.tab.rows), Cols: t.tab.cols()}).Region(
		tablemodel.Cell{Row: anchorRow, Col: anchorCol},
		tablemodel.Cell{Row: focusRow, Col: focusCol}, right,
	)
	return tableSelectionFromSnapshot(selection)
}

func (e *Editor) textSelectionForUnit(u *leaf) (int, int) {
	if e.block != nil {
		return 0, 0
	}
	if u.table != nil {
		// A table region owns selection presentation for the entire table. The
		// rectangle determines which cells are structurally selected; ordinary
		// source-range highlighting must not leak into the other cells.
		if e.tableStructureSelection(u.table) != nil {
			return 0, 0
		}
	}
	return e.textSelectionRange()
}

// leaveTableBoundary carries the selection envelope into an adjacent document
// block. Only the active head moves; the table is retained as a semantic part.
func (e *Editor) leaveTableBoundary(t *leaf, dir int, extend bool) {
	if !e.hasGoal {
		if r, ok := e.caretRect(e.selectionFocus()); ok {
			e.goalX, e.hasGoal = r.X, true
		}
	}
	for i, l := range e.leaves {
		if l != t {
			continue
		}
		if i+dir < 0 || i+dir >= len(e.leaves) {
			anchor := e.anchor
			if e.phantomAtEdge(i, dir) {
				if extend {
					e.anchor = anchor
				}
				return
			}
		}
		e.enterLeaf(i+dir, dir, extend)
		return
	}
}
