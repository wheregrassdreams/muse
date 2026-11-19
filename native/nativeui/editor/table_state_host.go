package editor

import (
	"time"

	editormodel "github.com/wheregrassdreams/muse-editor/text/session"
	tablemodel "github.com/wheregrassdreams/muse-editor/markdown/table"
	helix "github.com/wheregrassdreams/muse-editor/text/helix"
	tableui "muse/nativeui/editor/blocks/table"
)

type tableAxis = tablemodel.Axis

const (
	tableRows    = tablemodel.Rows
	tableColumns = tablemodel.Columns
	tableWhole   = tablemodel.Whole
	tableCells   = tablemodel.Cells
)

// tableSelection is a semantic selection. first/last are inclusive row or
// column indexes; a table selection uses the whole table.
type tableSelection struct {
	lineStart int
	// Cell coordinates are the canonical active table region. axis/first/last
	// remain compatibility fields for structural operations and legacy callers.
	anchorRow, anchorCol int
	focusRow, focusCol   int
	cellRegion           bool
	axis                 tableAxis
	first                int
	last                 int
	anchor               int
	active               int
	activeRow            int
	right                bool
	origin               tablemodel.SelectionOrigin
}

type tableContentBridge struct {
	lineStart       int
	axis            tableAxis
	row             int
	col             int
	dir             int
	resume          string
	index           int
	selectionAnchor int
	origin          tablemodel.SelectionOrigin
}

// tableNavigationState is the editor's immutable table state value. Text
// endpoints stay separate from table structure; a bridge owns its text focus.
func (e *Editor) tableNavigationState() tablemodel.Snapshot {
	return tablemodel.Snapshot{
		Mode: string(e.vimController().Mode()),
		TextSelection: editormodel.Selection{
			Anchor: e.Session.Selection().Anchor, Head: e.Caret(),
			Inclusive: e.Session.Selection().Inclusive, Linewise: e.Session.Selection().Linewise,
		},
		Selection:      tableSelectionSnapshot(e.tableSelection),
		DocumentRegion: tableSelectionSnapshot(e.tableDocumentRegion),
		Bridge:         tableBridgeSnapshot(e.tableBridge),
		VirtualEdit:    tableVirtualSnapshot(e.tableVirtualEdit),
		Visual:         tableVisualSnapshot(e),
	}
}

func tableVirtualSnapshot(s *tableVirtualEditState) *tablemodel.VirtualEdit {
	if s == nil {
		return nil
	}
	edge := tablemodel.VirtualBeforeCell
	if s.edge == tableVirtualTrailing {
		edge = tablemodel.VirtualAfterCell
	}
	return &tablemodel.VirtualEdit{LineStart: s.lineStart, Cell: tablemodel.Cell{Row: s.row, Col: s.col}, Edge: edge, Axis: tablemodel.Axis(s.axis), PostDelete: s.postDelete}
}

func tableVirtualFromSnapshot(s *tablemodel.VirtualEdit) *tableVirtualEditState {
	if s == nil {
		return nil
	}
	edge := tableVirtualLeading
	if s.Edge == tablemodel.VirtualAfterCell {
		edge = tableVirtualTrailing
	}
	return &tableVirtualEditState{lineStart: s.LineStart, row: s.Cell.Row, col: s.Cell.Col, edge: edge, axis: tableAxis(s.Axis), postDelete: s.PostDelete}
}

func tableVisualSnapshot(e *Editor) *tablemodel.VisualSelection {
	s := e.tableSelection
	if s == nil {
		return nil
	}
	mode := tablemodel.VisualModeNone
	if s.axis == tableRows && e.vimController().Mode() == helix.ModeVisual && e.vimController().View().Linewise {
		mode = tablemodel.VisualModeLine
	} else if s.axis == tableColumns && e.vimController().Mode() == helix.ModeBlockVisual {
		mode = tablemodel.VisualModeColumn
	}
	if mode == tablemodel.VisualModeNone {
		return nil
	}
	return &tablemodel.VisualSelection{Mode: mode, Anchor: tablemodel.Cell{Row: s.anchorRow, Col: s.anchorCol}, Focus: tablemodel.Cell{Row: s.focusRow, Col: s.focusCol}}
}

func (e *Editor) tableSnapshot() tablemodel.Snapshot { return e.tableNavigationState() }

func tableSelectionSnapshot(s *tableSelection) *tablemodel.Selection {
	if s == nil {
		return nil
	}
	return &tablemodel.Selection{
		LineStart:  s.lineStart,
		Anchor:     tablemodel.Cell{Row: s.anchorRow, Col: s.anchorCol},
		Focus:      tablemodel.Cell{Row: s.focusRow, Col: s.focusCol},
		CellRegion: s.cellRegion,
		Axis:       tablemodel.Axis(s.axis), First: s.first, Last: s.last,
		LegacyAnchor: s.anchor,
		Active:       s.active, ActiveRow: s.activeRow, Right: s.right,
		Origin: s.origin,
	}
}

func (e *Editor) tableTextOrigin() tablemodel.SelectionOrigin {
	switch e.vimController().Mode() {
	case helix.ModeVisual, helix.ModeSelect:
		return tablemodel.SelectionOriginVisualText
	case helix.ModeBlockVisual:
		return tablemodel.SelectionOriginTable
	default:
		return tablemodel.SelectionOriginText
	}
}

func tableSelectionFromSnapshot(s *tablemodel.Selection) *tableSelection {
	if s == nil {
		return nil
	}
	return &tableSelection{
		lineStart: s.LineStart,
		anchorRow: s.Anchor.Row, anchorCol: s.Anchor.Col,
		focusRow: s.Focus.Row, focusCol: s.Focus.Col,
		cellRegion: s.CellRegion,
		axis:       tableAxis(s.Axis), first: s.First, last: s.Last,
		anchor: s.LegacyAnchor,
		active: s.Active, activeRow: s.ActiveRow, right: s.Right,
		origin: s.Origin,
	}
}

func tableBridgeSnapshot(s *tableContentBridge) *tablemodel.ContentBridge {
	if s == nil {
		return nil
	}
	return &tablemodel.ContentBridge{
		LineStart: s.lineStart, Axis: tablemodel.Axis(s.axis), Row: s.row, Col: s.col,
		Direction: s.dir, Index: s.index,
		SelectionAnchor: s.selectionAnchor, Origin: s.origin, Resume: s.resume,
	}
}

func tableBridgeFromSnapshot(s *tablemodel.ContentBridge) *tableContentBridge {
	if s == nil {
		return nil
	}
	return &tableContentBridge{
		lineStart: s.LineStart, axis: tableAxis(s.Axis), row: s.Row, col: s.Col,
		dir: s.Direction, index: s.Index,
		selectionAnchor: s.SelectionAnchor, origin: s.Origin, resume: s.Resume,
	}
}

func (e *Editor) restoreTableSnapshot(s tablemodel.Snapshot) {
	e.restoreTableNavigation(s)
}

func sameTableNavigationState(a, b tablemodel.Snapshot) bool { return tablemodel.Equal(a, b) }

func (e *Editor) restoreTableNavigation(s tablemodel.Snapshot) {
	e.modalSetMode(helix.Mode(s.Mode))
	e.setSessionSelection(s.TextSelection.Anchor, s.TextSelection.Head)
	e.setSessionInclusive(s.TextSelection.Inclusive)
	e.setSessionLinewise(s.TextSelection.Linewise)
	e.tableSelection, e.tableBridge = nil, nil
	e.tableDocumentRegion = nil
	if s.Selection != nil {
		e.tableSelection = tableSelectionFromSnapshot(s.Selection)
	}
	if s.Bridge != nil {
		e.tableBridge = tableBridgeFromSnapshot(s.Bridge)
	}
	if s.DocumentRegion != nil {
		e.tableDocumentRegion = tableSelectionFromSnapshot(s.DocumentRegion)
	}
	e.tableVirtualEdit = tableVirtualFromSnapshot(s.VirtualEdit)
	e.hasGoal = false
	e.wantReveal = true
	e.blinkStart = time.Now()
	e.invalidate()
}

// replayTableNavigation restores the complete prior selection state when the
// latest cell transition is followed by its inverse direction.
func (e *Editor) replayTableNavigation(rowDelta, colDelta int) bool {
	result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.ReplayNavigation, BeforeState: e.tableNavigationState(), RowDelta: rowDelta, ColDelta: colDelta})
	if err == nil && result.Found {
		e.restoreTableNavigation(result.Snapshot)
		return true
	}
	return false
}

func (e *Editor) recordTableNavigation(before tablemodel.Snapshot, rowDelta, colDelta int) {
	_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.RecordNavigation, BeforeState: before, AfterState: e.tableNavigationState(), RowDelta: rowDelta, ColDelta: colDelta})
}

// selectionFocus is the active variant's focus, never the dormant text head.
func (e *Editor) selectionFocus() int {
	s, t := e.tableSelection, e.tableSelectionLeaf()
	if s != nil && t != nil {
		if s.axis == tableCells {
			row := max(0, min(s.focusRow, len(t.tab.rows)-1))
			col := max(0, min(s.focusCol, len(t.tab.rows[row])-1))
			cell := t.tab.rows[row][col]
			return cell.start
		}
		if s.axis == tableWhole {
			if s.right {
				return blockEnd(e.Source(), t)
			}
			return t.lineStart
		}
		if s.axis == tableRows {
			return e.tableRowBoundary(t, s.active, s.right)
		}
		if s.axis == tableColumns {
			row := t.tab.rows[max(0, min(s.activeRow, len(t.tab.rows)-1))]
			cell := row[max(0, min(s.active, len(row)-1))]
			if s.right {
				return cell.end
			}
			return cell.start
		}
	}
	return e.Caret()
}

type tableBlockInsert struct {
	lineStart int
	firstCol  int
	rows      []int
	text      string
}
