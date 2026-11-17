package table

import "muse/editor"

// SelectionOrigin records the semantic interaction that created a table
// selection without importing or persisting a particular keymap's mode.
type SelectionOrigin uint8

const (
	SelectionOriginTable SelectionOrigin = iota
	SelectionOriginText
	SelectionOriginVisualText
)

// Clamp projects a structural selection onto the current table grid and
// recomputes its semantic axis and inclusive range while retaining its input
// origin metadata.
func (s Selection) Clamp(grid Grid) (Selection, bool) {
	if !s.CellRegion || s.LineStart != grid.LineStart {
		return Selection{}, false
	}
	selection := grid.Region(s.Anchor, s.Focus, s.Right)
	if selection == nil {
		return Selection{}, false
	}
	selection.Origin = s.Origin
	return *selection, true
}

type ContentBridge struct {
	LineStart       int
	Axis            Axis
	Row             int
	Col             int
	Direction       int
	Index           int
	SelectionAnchor int
	Origin          SelectionOrigin
	Resume          string
}

// VirtualEdge identifies the side of a cell at which a structural edit has
// temporarily placed the caret. It is source-independent session state; the
// host resolves it to a concrete source offset through a DocumentView.
type VirtualEdge uint8

const (
	VirtualBeforeCell VirtualEdge = iota + 1
	VirtualAfterCell
)

// VirtualEdit is a temporary table caret position used when a command moves
// past the source-backed edge of a row or column. It contains only stable
// table coordinates and can be copied into an editor history snapshot.
type VirtualEdit struct {
	LineStart  int
	Cell       Cell
	Edge       VirtualEdge
	Axis       Axis
	PostDelete bool
}

// Clamp constrains the virtual position to the current table grid. It returns
// false when the grid is empty or the edge is not a valid virtual edge.
func (v VirtualEdit) Clamp(grid Grid) (VirtualEdit, bool) {
	if grid.Rows <= 0 || grid.Cols <= 0 ||
		(v.Edge != VirtualBeforeCell && v.Edge != VirtualAfterCell) {
		return VirtualEdit{}, false
	}
	v.LineStart = grid.LineStart
	v.Cell.Row = clamp(v.Cell.Row, 0, grid.Rows-1)
	v.Cell.Col = clamp(v.Cell.Col, 0, grid.Cols-1)
	return v, true
}

// ValidateVirtualEdit confirms that the stored source anchor still resolves
// to the same concrete cell in a fresh parser projection. It never guesses a
// replacement table or clamps a stale row/column to a different cell.
func ValidateVirtualEdit(view DocumentView, virtual VirtualEdit) (VirtualEdit, bool) {
	if virtual.LineStart != view.LineStart || virtual.Cell.Row < 0 || virtual.Cell.Row >= len(view.Rows) ||
		virtual.Cell.Col < 0 || virtual.Cell.Col >= len(view.Rows[virtual.Cell.Row]) {
		return VirtualEdit{}, false
	}
	if virtual.Edge != VirtualBeforeCell && virtual.Edge != VirtualAfterCell {
		return VirtualEdit{}, false
	}
	if virtual.Axis != Rows && virtual.Axis != Columns && virtual.Axis != Whole && virtual.Axis != Cells {
		return VirtualEdit{}, false
	}
	return virtual, true
}

// VisualMode is a table-local extension of editor visual selection.
type VisualMode uint8

const (
	VisualModeNone VisualMode = iota
	VisualModeLine
	VisualModeColumn
)

// VisualSelection stores table coordinates for a visual selection without
// retaining renderer geometry or a mutable parser leaf.
type VisualSelection struct {
	Mode   VisualMode
	Anchor Cell
	Focus  Cell
}

// Clamp constrains both endpoints to the current grid and rejects an empty
// grid or an unsupported mode.
func (v VisualSelection) Clamp(grid Grid) (VisualSelection, bool) {
	if grid.Rows <= 0 || grid.Cols <= 0 || v.Mode < VisualModeLine || v.Mode > VisualModeColumn {
		return VisualSelection{}, false
	}
	v.Anchor.Row = clamp(v.Anchor.Row, 0, grid.Rows-1)
	v.Anchor.Col = clamp(v.Anchor.Col, 0, grid.Cols-1)
	v.Focus.Row = clamp(v.Focus.Row, 0, grid.Rows-1)
	v.Focus.Col = clamp(v.Focus.Col, 0, grid.Cols-1)
	return v, true
}

// Snapshot captures table-specific selection alongside the editor's text
// selection. It is the single value stored by editor undo/redo for table state.
type Snapshot struct {
	Mode           string
	TextSelection  editor.Selection
	Selection      *Selection
	DocumentRegion *Selection
	Bridge         *ContentBridge
	VirtualEdit    *VirtualEdit
	Visual         *VisualSelection
}

type navigationStep struct {
	before, after Snapshot
	rowDelta      int
	colDelta      int
}

// Controller owns table-specific navigation replay state. The editor supplies
// immutable snapshots before/after a movement and applies any restored value.
type Controller struct {
	navigation []navigationStep
}
