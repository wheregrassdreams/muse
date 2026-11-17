package editor

import (
	"github.com/egoist/mygo/ui"
	tablemodel "muse/editor/blocks/table"
	"muse/editor/input/vim"
)

func (e *Editor) tableSelectionSourceRange(t *leaf, s *tableSelection) (int, int, bool) {
	if t == nil || s == nil || t.tab == nil {
		return 0, 0, false
	}
	effective, ok := tablemodel.EffectiveSelection(
		documentViewForTable(e, t), e.Session.Selection(), tableSelectionSnapshot(s), nil, e.block != nil,
	)
	if !ok {
		return 0, 0, false
	}
	range_, ok := effective.SourceRange(documentViewForTable(e, t))
	return range_.Start, range_.End, ok
}

func (e *Editor) tableRowBoundary(t *leaf, row int, right bool) int {
	lines := e.tableLines(t)
	line := row
	if row > 0 {
		line = row + 1
	}
	if line < 0 || line >= len(lines) {
		return t.lineStart
	}
	if right {
		return lines[line][1]
	}
	return lines[line][0]
}

func (e *Editor) tableSelectionContainsCell(t *leaf, row, col int) bool {
	return tableSelectionContainsCell(e.tableSelection, t, row, col)
}

func tableSelectionContainsCell(s *tableSelection, t *leaf, row, col int) bool {
	if s == nil || t == nil || s.lineStart != t.lineStart {
		return false
	}
	selection := tableSelectionSnapshot(s)
	return selection != nil && selection.Contains(tablemodel.Cell{Row: row, Col: col})
}

// tableSelectionContentRange returns the rendered content span of a structural
// selection. It lets a structural selection downgrade into one ordinary text
// selection when horizontal motion enters adjacent content.
func (e *Editor) tableSelectionContentRange(t *leaf, s *tableSelection) (int, int) {
	if t == nil || s == nil || t.tab == nil || len(t.tab.rows) == 0 {
		return 0, 0
	}
	range_, ok := tableSelectionSnapshot(s).ContentRange(documentViewForTable(e, t))
	if !ok {
		return 0, 0
	}
	return range_.Start, range_.End
}

func (e *Editor) tableSelectionCaretRect() (ui.Rect, bool) {
	s := e.tableSelection
	t := e.tableSelectionLeaf()
	if s == nil || t == nil || t.geom == nil || len(t.geom.rowY) < 2 {
		return ui.Rect{}, false
	}
	row := 0
	x := tableContentX(t)
	if s.cellRegion {
		row = max(0, min(s.focusRow, min(len(t.tab.rows), len(t.geom.rowY)-1)-1))
		col := max(0, min(s.focusCol, len(t.geom.colX)-2))
		x += t.geom.colX[col]
	} else if s.axis == tableColumns {
		col := max(0, min(s.active, len(t.geom.colX)-2))
		if s.right {
			col++
		}
		x += t.geom.colX[col]
		row = max(0, min(s.activeRow, min(len(t.tab.rows), len(t.geom.rowY)-1)-1))
	} else {
		row = max(0, min(s.active, min(len(t.tab.rows), len(t.geom.rowY)-1)-1))
		if s.axis == tableWhole {
			row = 0
			if s.right {
				row = len(t.tab.rows) - 1
			}
		}
		if s.right {
			x += t.geom.w
		}
	}
	return ui.Rect{X: x, Y: t.y + t.geom.rowY[row], W: 1, H: t.geom.rowY[row+1] - t.geom.rowY[row]}, true
}

func (e *Editor) tableBoundaryAtOffset(off int) (*leaf, int, bool, bool) {
	if e.phantom != nil && e.leafIsPhantom(off) {
		return nil, 0, false, false
	}
	for _, t := range e.leaves {
		if t.kind != kTable || t.tab == nil || t.geom == nil {
			continue
		}
		lines := e.tableLines(t)
		for row := range t.tab.rows {
			li := row
			if row > 0 {
				li++
			}
			if li >= len(lines) || row+1 >= len(t.geom.rowY) {
				continue
			}
			if off == lines[li][0] {
				return t, row, false, true
			}
			if off == lines[li][1] {
				return t, row, true, true
			}
		}
	}
	return nil, 0, false, false
}

func (e *Editor) moveStructuralRow(dir int, extend bool) bool {
	s := e.tableSelection
	if s == nil || (s.axis != tableRows && s.axis != tableWhole) {
		return false
	}
	t := e.tableSelectionLeaf()
	if t == nil {
		return false
	}
	active := s.active + dir
	if s.axis == tableWhole || active < 0 || active >= len(t.tab.rows) {
		focus := e.selectionFocus()
		a, z := e.selection()
		e.clearTableSelection()
		e.setSessionHead(focus)
		e.setSessionAnchor(focus)
		if extend {
			e.setSessionAnchor(a)
			if dir < 0 {
				e.setSessionAnchor(z)
			}
		}
		e.leaveTableBoundary(t, dir, extend)
		return true
	}
	anchor := active
	if extend {
		anchor = s.anchor
	}
	right, origin := s.right, s.origin
	e.selectTableAxis(t, tableRows, anchor, active, false)
	e.tableSelection.right = right
	if extend {
		e.tableSelection.origin = origin
	}
	return true
}

func (e *Editor) structuralSelectionForRange() bool {
	// Recompute from the current endpoints on every extended move. Keeping a
	// previous structural state here made a shrinking selection stay structural.
	suspended := cloneTableSelection(e.tableDocumentRegion)
	e.tableSelection = nil
	if e.Session.Selection().Anchor == e.Caret() {
		return false
	}
	for _, t := range e.leaves {
		if t.kind != kTable || t.tab == nil {
			continue
		}
		// When a text-origin rectangle's head leaves the table, preserve its
		// exact cell focus while the ordinary source range grows outside. A
		// fresh projection from the expanded envelope would move the table
		// focus to the row touched by the document endpoint.
		if suspended != nil && suspended.lineStart == t.lineStart &&
			(e.Session.Selection().Anchor < t.lineStart || e.Session.Selection().Anchor > blockEnd(e.Source(), t) ||
				e.Caret() < t.lineStart || e.Caret() > blockEnd(e.Source(), t)) {
			start, end := e.selection()
			if start < blockEnd(e.Source(), t) && end > t.lineStart {
				e.tableDocumentRegion = suspended
				return true
			}
		}
		// Keep a table's projected structural selection alongside the document
		// envelope when the endpoints extend into surrounding text. The domain
		// splitter owns the promotion rule; the host only stores its snapshot.
		if e.block == nil && e.vimController().Mode() != vim.ModeBlockVisual {
			selection := e.Session.Selection()
			selection.Head = e.Caret()
			parts := tablemodel.ProjectDocumentSelection(selection, []tablemodel.DocumentView{documentViewForTable(e, t)})
			for _, part := range parts {
				if part.Table != nil && (selection.Anchor < t.lineStart || selection.Anchor > blockEnd(e.Source(), t) || selection.Head < t.lineStart || selection.Head > blockEnd(e.Source(), t)) {
					part.Table.Origin = e.tableTextOrigin()
					e.tableDocumentRegion = tableSelectionFromSnapshot(part.Table)
					return true
				}
			}
		}
		// A region suspended while the Visual caret was outside its table is
		// part of the same selection session. Re-entering the table resumes it
		// from the saved anchor and the parser-owned cell containing the caret;
		// the document endpoints continue to own any text outside the table.
		if suspended != nil && suspended.origin != tablemodel.SelectionOriginTable && suspended.lineStart == t.lineStart {
			focusTable, focusRow, focusCol, focusOK := e.tableCellAtSourceOffset(e.Caret())
			if focusOK && focusTable.lineStart == t.lineStart {
				if e.selectTableCellRegion(t, suspended.anchorRow, suspended.anchorCol, focusRow, focusCol) {
					e.tableSelection.origin = suspended.origin
					e.tableDocumentRegion = nil
					return true
				}
			}
		}
		lines := e.tableLines(t)
		// A document selection can contain text and table structures together.
		// Do not replace that envelope with a single row at the active edge.
		if e.Session.Selection().Anchor < t.lineStart || e.Session.Selection().Anchor > blockEnd(e.Source(), t) || e.Caret() < t.lineStart || e.Caret() > blockEnd(e.Source(), t) {
			continue
		}
		rowAtOffset := func(off int) (int, bool) {
			for row := range t.tab.rows {
				li := row
				if row > 0 {
					li = row + 1
				}
				if li < len(lines) && off >= lines[li][0] && off <= lines[li][1] {
					return row, true
				}
			}
			return 0, false
		}
		arow, aok := rowAtOffset(e.Session.Selection().Anchor)
		crow, cok := rowAtOffset(e.Caret())
		if aok && cok {
			rowBoundary := func(row, off int) bool {
				li := row
				if row > 0 {
					li = row + 1
				}
				return li < len(lines) && (off == lines[li][0] || off == lines[li][1])
			}
			if rowBoundary(arow, e.Session.Selection().Anchor) || rowBoundary(crow, e.Caret()) {
				first, last := min(arow, crow), max(arow, crow)
				li := crow
				if crow > 0 {
					li++
				}
				right := li < len(lines) && e.Caret() == lines[li][1]
				s := &tableSelection{lineStart: t.lineStart, axis: tableRows, first: first, last: last, anchor: arow, active: crow, activeRow: crow, right: right, origin: e.tableTextOrigin()}
				e.tableSelection = s
				return true
			}
		}
		for r := range t.tab.rows {
			li := r
			if r > 0 {
				li = r + 1
			}
			if li >= len(lines) {
				continue
			}
			ls, le := lines[li][0], lines[li][1]
			anchorLine, caretLine := lineStartAt(e.Source(), e.Session.Selection().Anchor), lineStartAt(e.Source(), e.Caret())
			touches := e.Session.Selection().Anchor == ls || e.Session.Selection().Anchor == le || e.Caret() == ls || e.Caret() == le
			if touches && (anchorLine == ls || caretLine == ls) {
				s := &tableSelection{lineStart: t.lineStart, axis: tableRows, first: r, last: r, anchor: r, active: r, activeRow: r, origin: e.tableTextOrigin()}
				e.tableSelection = s
				return true
			}
		}
		_, ar, ac := cellAt(t, e.Session.Selection().Anchor)
		_, cr, cc := cellAt(t, e.Caret())
		if e.vimController().Mode() == vim.ModeBlockVisual && ar >= 0 && cr >= 0 && ar != cr {
			lastRow := len(t.tab.rows) - 1
			topCol, bottomCol := -1, -1
			if ar == 0 && e.Session.Selection().Anchor <= t.tab.rows[ar][ac].start && cr == lastRow && e.Caret() >= t.tab.rows[cr][cc].end {
				topCol, bottomCol = ac, cc
			} else if cr == 0 && e.Caret() <= t.tab.rows[cr][cc].start && ar == lastRow && e.Session.Selection().Anchor >= t.tab.rows[ar][ac].end {
				topCol, bottomCol = cc, ac
			}
			if topCol >= 0 {
				first, last := min(topCol, bottomCol), max(topCol, bottomCol)
				right := e.Caret() >= t.tab.rows[cr][cc].end
				e.tableSelection = &tableSelection{lineStart: t.lineStart, axis: tableColumns, first: first, last: last, anchor: topCol, active: bottomCol, activeRow: cr, right: right, origin: e.tableTextOrigin()}
				return true
			}
		}
	}
	return false
}
