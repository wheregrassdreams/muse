package table

import "muse/editor"

// SelectionPart is one semantic segment of a document text selection. Text
// outside a table remains a source range; a crossed table becomes one
// structural selection over its parser-projected cells.
type SelectionPart struct {
	Source editor.TextRange
	Table  *Selection
}

// SelectionForTextRange promotes a source selection to a table region only
// when it crosses a table boundary or covers the whole table. A selection
// wholly inside table source remains an ordinary text selection.
func SelectionForTextRange(view DocumentView, text editor.Selection) (*Selection, bool) {
	if !validView(view) || text.Anchor == text.Head {
		return nil, false
	}
	start, end := ordered(text.Anchor, text.Head)
	if start >= view.End || end <= view.LineStart {
		return nil, false
	}
	if !(start <= view.LineStart && end >= view.End) && start >= view.LineStart && end <= view.End {
		return nil, false
	}
	if len(view.Rows) == 0 || maxRowWidth(view.Rows) == 0 {
		return nil, false
	}

	anchor := Cell{Row: 0, Col: 0}
	if start > view.LineStart {
		var ok bool
		anchor, ok = CellAtSourceOffset(view, start, PreferNextCell)
		if !ok {
			return nil, false
		}
	}

	lastRow := len(view.Rows) - 1
	focus := Cell{Row: lastRow, Col: max(0, len(view.Rows[lastRow])-1)}
	if end < view.End {
		var ok bool
		focus, ok = CellAtSourceOffset(view, end-1, PreferPreviousCell)
		if !ok {
			return nil, false
		}
	}
	right := text.Head >= text.Anchor
	if !right {
		anchor, focus = focus, anchor
	}
	region := (Grid{LineStart: view.LineStart, Rows: len(view.Rows), Cols: maxRowWidth(view.Rows)}).Region(anchor, focus, right)
	return region, region != nil
}

// EffectiveSelection resolves table-local selection ownership from the
// session's structural selection snapshots and the current source selection.
// A block selection suppresses implicit promotion, while explicit stored
// selections remain authoritative.
func EffectiveSelection(view DocumentView, text editor.Selection, selected, documentRegion *Selection, blockSelection bool) (*Selection, bool) {
	if !validView(view) {
		return nil, false
	}
	if selected != nil && selected.LineStart == view.LineStart {
		copy := *selected
		return &copy, true
	}
	if documentRegion != nil && documentRegion.LineStart == view.LineStart {
		start, end := ordered(text.Anchor, text.Head)
		if start < view.End && end > view.LineStart {
			copy := *documentRegion
			return &copy, true
		}
	}
	if blockSelection {
		return nil, false
	}
	return SelectionForTextRange(view, text)
}

// PromoteCellRegion creates the canonical structural selection used when a
// text selection is promoted to cell selection or when the user selects a
// cell range directly. Coordinate resolution and mode eligibility remain with
// the input adapter; orientation and table selection semantics live here.
func PromoteCellRegion(grid Grid, anchor, focus Cell, right bool, origin SelectionOrigin) (*Selection, bool) {
	if origin > SelectionOriginVisualText {
		return nil, false
	}
	selection := grid.Region(anchor, focus, right)
	if selection == nil {
		return nil, false
	}
	selection.Origin = origin
	return selection, true
}

// RegionMove is the result of moving the active end of a structural table
// selection. Exited is true when movement crossed a table edge; the host then
// resolves ExitCell to a source caret and continues document navigation.
type RegionMove struct {
	Selection Selection
	Handled   bool
	Changed   bool
	Exited    bool
	ExitCell  Cell
	Direction Cell
}

// MoveRegion advances a cell-region focus by one or more grid coordinates.
// It returns edge information instead of choosing a source offset or moving
// between UI blocks. The anchor and text-promotion identity are preserved.
func MoveRegion(grid Grid, current Selection, rowDelta, colDelta int) RegionMove {
	if grid.Rows <= 0 || grid.Cols <= 0 || !current.CellRegion ||
		current.LineStart != grid.LineStart || (rowDelta == 0 && colDelta == 0) {
		return RegionMove{}
	}
	current, ok := current.Clamp(grid)
	if !ok {
		return RegionMove{}
	}
	row, col := current.Focus.Row+rowDelta, current.Focus.Col+colDelta
	if row < 0 || row >= grid.Rows || col < 0 || col >= grid.Cols {
		return RegionMove{
			Handled: true, Exited: true,
			ExitCell:  Cell{Row: clamp(current.Focus.Row, 0, grid.Rows-1), Col: clamp(current.Focus.Col, 0, grid.Cols-1)},
			Direction: Cell{Row: sign(rowDelta), Col: sign(colDelta)},
		}
	}
	selection := grid.Region(current.Anchor, Cell{Row: row, Col: col}, current.Right)
	if selection == nil {
		return RegionMove{}
	}
	selection.Origin = current.Origin
	return RegionMove{
		Selection: *selection,
		Handled:   true,
		Changed:   selection.Focus != current.Focus || selection.Anchor != current.Anchor,
	}
}

func sign(value int) int {
	if value < 0 {
		return -1
	}
	if value > 0 {
		return 1
	}
	return 0
}

// ProjectDocumentSelection decomposes a source selection across ordered table
// projections. Each intersected table that is crossed structurally is emitted
// once as a whole-source envelope plus a rectangular table selection. Tables
// wholly inside the selection's active text range remain in a text segment.
// The views must belong to the same immutable source revision and be ordered
// by LineStart.
func ProjectDocumentSelection(text editor.Selection, tables []DocumentView) []SelectionPart {
	start, end := ordered(text.Anchor, text.Head)
	if start == end {
		return nil
	}
	parts := make([]SelectionPart, 0, len(tables)*2+1)
	cursor := start
	for _, view := range tables {
		if !validView(view) || view.LineStart >= end || view.End <= start || view.LineStart < cursor && cursor != start {
			continue
		}
		region, ok := SelectionForTextRange(view, text)
		if !ok {
			continue
		}
		if cursor < view.LineStart {
			parts = append(parts, SelectionPart{Source: editor.TextRange{Start: cursor, End: view.LineStart}})
		}
		parts = append(parts, SelectionPart{
			Source: editor.TextRange{Start: view.LineStart, End: view.End},
			Table:  region,
		})
		cursor = view.End
	}
	if cursor < end {
		parts = append(parts, SelectionPart{Source: editor.TextRange{Start: cursor, End: end}})
	}
	return parts
}

func validView(view DocumentView) bool {
	return view.LineStart >= 0 && view.End >= view.LineStart && view.End <= len(view.Source)
}

func maxRowWidth(rows [][]editor.TextRange) int {
	width := 0
	for _, row := range rows {
		width = max(width, len(row))
	}
	return width
}

func ordered(a, b int) (int, int) {
	if a <= b {
		return a, b
	}
	return b, a
}
