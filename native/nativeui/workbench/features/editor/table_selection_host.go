package editor

import (
	"time"

	tablemodel "muse/editor/blocks/table"
	tableui "muse/nativeui/workbench/features/editor/blocks/table"
)

func (e *Editor) tableSelectionLeaf() *leaf {
	if e.tableSelection == nil {
		return nil
	}
	for _, l := range e.leaves {
		if l.kind == kTable && l.lineStart == e.tableSelection.lineStart && l.tab != nil {
			return l
		}
	}
	return nil
}

// tablePositionAtSource maps parser-owned table line boundaries to structural
// coordinates. The source offset identifies a parsed line only; row and cell
// shape still come from the table projection, never from pipe or cell text.
func (e *Editor) tablePositionAtSource(t *leaf, off int) (row, col int, ok bool) {
	if t == nil || t.tab == nil || len(t.tab.rows) == 0 {
		return 0, 0, false
	}
	cell, ok := tablemodel.CellAtSourceOffset(documentViewForTable(e, t), off, tablemodel.PreferNextCell)
	return cell.Row, cell.Col, ok
}

func (e *Editor) selectTableAxis(t *leaf, axis tableAxis, first, last int, extend bool) {
	if t == nil {
		return
	}
	_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.SelectAxis, LineStart: t.lineStart, Axis: tablemodel.Axis(axis), First: first, Last: last, Extend: extend})
}

func (e *Editor) applyTableAxis(t *leaf, axis tableAxis, first, last int, extend bool) {
	if e.block != nil {
		e.block = nil
	}
	e.finishBlockInput()
	if !e.tableNavigating {
		e.resetTableNavigation()
	}
	if t == nil || t.tab == nil {
		return
	}
	limit := len(t.tab.rows)
	if axis == tableColumns {
		limit = t.tab.cols()
	} else if axis == tableWhole {
		first, last = 0, max(limit-1, 0)
	}
	if limit == 0 {
		return
	}
	anchor := max(0, min(first, limit-1))
	active := max(0, min(last, limit-1))
	first, last = max(0, min(first, limit-1)), max(0, min(last, limit-1))
	if first > last {
		first, last = last, first
	}
	if extend && e.tableSelection != nil && e.tableSelection.lineStart == t.lineStart && e.tableSelection.axis == axis {
		anchor = e.tableSelection.anchor
		last = max(0, min(last, limit-1))
		first = min(anchor, last)
		last = max(anchor, last)
	}
	region := (tablemodel.Grid{LineStart: t.lineStart, Rows: len(t.tab.rows), Cols: t.tab.cols()}).AxisRegion(axis, anchor, active, true)
	if region == nil {
		return
	}
	s := tableSelectionFromSnapshot(region)
	// Axis selections predate cell-region selections and retain their mode
	// marker; both shapes share the domain's canonical endpoints and bounds.
	s.cellRegion = false
	s.activeRow = 0
	if axis == tableRows {
		s.activeRow = active
	}
	e.tableSelection = s
	e.tableBridge = nil
	e.clearComposition()
	e.hasGoal = false
	e.wantReveal = true
	e.blinkStart = time.Now()
	e.invalidate()
}

// selectTableCellRegion updates the one active table selection using parser
// cell coordinates. It intentionally leaves ordinary text endpoints dormant.
func (e *Editor) selectTableCellRegion(t *leaf, anchorRow, anchorCol, focusRow, focusCol int) bool {
	if t == nil {
		return false
	}
	result, err := e.tableUI().Dispatch(tableui.Action{
		Kind: tableui.SelectCell, LineStart: t.lineStart,
		Anchor: tablemodel.Cell{Row: anchorRow, Col: anchorCol},
		Focus:  tablemodel.Cell{Row: focusRow, Col: focusCol},
	})
	return err == nil && result.Changed
}

func (e *Editor) applyTableCellRegion(t *leaf, anchorRow, anchorCol, focusRow, focusCol int) bool {
	if t == nil || t.tab == nil || len(t.tab.rows) == 0 || t.tab.cols() == 0 {
		return false
	}
	region, ok := tablemodel.PromoteCellRegion(
		tablemodel.Grid{LineStart: t.lineStart, Rows: len(t.tab.rows), Cols: t.tab.cols()},
		tablemodel.Cell{Row: anchorRow, Col: anchorCol},
		tablemodel.Cell{Row: focusRow, Col: focusCol}, true, e.tableTextOrigin(),
	)
	if !ok {
		return false
	}
	e.clearTableSelectionSession()
	// The region owns table presentation while the document endpoints retain
	// any selected text outside the table. Clear incompatible linewise/text flags.
	e.setSessionInclusive(false)
	e.setSessionLinewise(false)
	e.vimController().SetLinewise(false)
	e.tableSelection = &tableSelection{
		lineStart: region.LineStart,
		axis:      tableAxis(region.Axis),
		anchorRow: region.Anchor.Row, anchorCol: region.Anchor.Col,
		focusRow: region.Focus.Row, focusCol: region.Focus.Col,
		cellRegion: region.CellRegion, first: region.First, last: region.Last,
		anchor: region.LegacyAnchor, active: region.Active, activeRow: region.ActiveRow,
		right: region.Right, origin: region.Origin,
	}
	e.tableBridge = nil
	if !e.tableNavigating {
		e.resetTableNavigation()
	}
	e.wantReveal = true
	e.invalidate()
	return true
}

func (e *Editor) resetTableNavigation() {
	_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.ResetNavigation})
}

func (e *Editor) tableSelectionCellRegion(s *tableSelection) (tablemodel.Selection, bool) {
	if s == nil || !s.cellRegion {
		return tablemodel.Selection{}, false
	}
	selection := tableSelectionSnapshot(s)
	if selection == nil {
		return tablemodel.Selection{}, false
	}
	return *selection, true
}

// moveTableCellRegion delegates structural movement to the table domain and
// installs the resulting immutable selection snapshot in the UI host.
func (e *Editor) moveTableCellRegion(rowDelta, colDelta int) bool {
	if e.replayTableNavigation(rowDelta, colDelta) {
		return true
	}
	selection := e.tableSelection
	if selection == nil || !selection.cellRegion {
		return false
	}
	t := e.tableSelectionLeaf()
	if t == nil || t.tab == nil {
		return false
	}
	before := e.tableNavigationState()
	grid := tablemodel.Grid{LineStart: t.lineStart, Rows: len(t.tab.rows), Cols: t.tab.cols()}
	move := tablemodel.MoveRegion(grid, *tableSelectionSnapshot(selection), rowDelta, colDelta)
	if !move.Handled {
		return false
	}
	if move.Exited {
		// A text-origin cell rectangle moves horizontally between rows at the
		// same column. Table-origin rectangles keep the traditional behavior:
		// crossing any outer edge resumes ordinary document selection.
		if rowDelta == 0 && colDelta != 0 && selection.origin == tablemodel.SelectionOriginVisualText {
			nextRow := selection.focusRow
			if colDelta < 0 {
				nextRow--
			} else {
				nextRow++
			}
			if nextRow >= 0 && nextRow < grid.Rows {
				if wrapped, ok := tablemodel.PromoteCellRegion(grid,
					tablemodel.Cell{Row: selection.anchorRow, Col: selection.anchorCol},
					tablemodel.Cell{Row: nextRow, Col: selection.focusCol}, selection.right, selection.origin); ok {
					e.tableSelection = tableSelectionFromSnapshot(wrapped)
					e.tableDocumentRegion = nil
					e.hasGoal = false
					e.wantReveal = true
					e.invalidate()
					e.recordTableNavigation(before, rowDelta, colDelta)
					return true
				}
			}
		}

		// Keep the last cell rectangle as a structural part while extending the
		// source selection into the neighboring document text.
		e.tableSelection = nil
		e.tableBridge = nil
		e.tableDocumentRegion = cloneTableSelection(selection)
		direction := -1
		if move.Direction.Row > 0 || move.Direction.Col > 0 {
			direction = 1
		}
		e.tableNavigating = true
		e.leaveTableBoundary(t, direction, true)
		e.tableNavigating = false
		if selection.origin == tablemodel.SelectionOriginVisualText {
			e.vimController().SetTextVisualActive(false)
		}
		// The source envelope can temporarily map to a row-shaped selection.
		// Keep the cell rectangle as a suspended document part instead.
		e.tableSelection = nil
		e.tableDocumentRegion = cloneTableSelection(selection)
		e.hasGoal = false
		e.wantReveal = true
		e.invalidate()
		e.recordTableNavigation(before, rowDelta, colDelta)
		return true
	}
	e.tableSelection = tableSelectionFromSnapshot(&move.Selection)
	e.tableDocumentRegion = nil
	e.hasGoal = false
	e.wantReveal = true
	e.blinkStart = time.Now()
	e.invalidate()
	e.recordTableNavigation(before, rowDelta, colDelta)
	return true
}

// tableHorizontalTransition exposes adjacent cell text first. Continuing to
// the far edge promotes the traversed row or column to a structure.
