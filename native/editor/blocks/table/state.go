// Package table owns the semantic state contract for block-table editing.
// The editor facade stores history snapshots of these values; view and source
// models are deliberately absent from this package.
package table

import (
	"strings"

	"muse/editor"
)

// Host is the narrow bridge from table commands to the editor document. Views
// are immutable and short lived; writes are atomic editor transactions.
type Host interface {
	TableView(lineStart int) (DocumentView, bool)
	DocumentSource() string
	TextSelection() editor.Selection
	Apply(editor.Transaction) error
}

type Axis uint8

const (
	Rows Axis = iota + 1
	Columns
	Whole
	Cells
)

type Cell struct {
	Row int
	Col int
}

// Grid is the immutable shape of a parsed table, expressed without parser or
// rendering types. Cols is the maximum row width in the source projection.
type Grid struct {
	LineStart int
	Rows      int
	Cols      int
}

// DocumentView is a short-lived, read-only projection of parser-owned table
// cell byte ranges into the existing immutable Markdown source string. It
// keeps no parser or renderer objects and must not be retained across edits.
type DocumentView struct {
	LineStart          int
	End                int
	Source             string
	Prefix             string
	ContinuationPrefix string
	Rows               [][]editor.TextRange
	Lines              []LineView
	Alignments         []Alignment
}

// LineView contains a table source line and parser-computed unescaped pipe
// offsets, allowing commands to preserve whitespace and escaped delimiters.
type LineView struct {
	Range editor.TextRange
	Pipes []int
}

// BoundaryAffinity selects which adjacent cell owns a caret at a shared
// source boundary.
type BoundaryAffinity uint8

const (
	PreferNextCell BoundaryAffinity = iota
	PreferPreviousCell
)

// CellAtSourceOffset maps a source caret to a parser-projected cell. Cell
// ranges are half-open; affinity only affects a caret exactly at a cell end.
func CellAtSourceOffset(view DocumentView, offset int, affinity BoundaryAffinity) (Cell, bool) {
	if offset < view.LineStart || offset > view.End ||
		(affinity != PreferNextCell && affinity != PreferPreviousCell) {
		return Cell{}, false
	}
	var previous Cell
	previousEnd := -1
	for rowIndex, row := range view.Rows {
		for colIndex, cell := range row {
			if cell.Start < view.LineStart || cell.End < cell.Start || cell.End > view.End {
				return Cell{}, false
			}
			if offset == cell.Start && affinity == PreferPreviousCell && previousEnd == offset {
				return previous, true
			}
			if offset >= cell.Start && offset < cell.End {
				return Cell{Row: rowIndex, Col: colIndex}, true
			}
			if cell.End == offset {
				previous = Cell{Row: rowIndex, Col: colIndex}
				previousEnd = cell.End
			}
		}
	}
	if affinity == PreferPreviousCell && previousEnd == offset {
		return previous, true
	}
	for lineIndex, line := range view.Lines {
		if offset < line.Range.Start || offset > line.Range.End {
			continue
		}
		row := 0
		if lineIndex >= 2 {
			row = min(lineIndex-1, len(view.Rows)-1)
		}
		if row < 0 || row >= len(view.Rows) || len(view.Rows[row]) == 0 {
			return Cell{}, false
		}
		if lineIndex == 1 {
			// The Markdown alignment separator belongs to the header boundary.
			return Cell{Row: 0, Col: 0}, true
		}
		cells := view.Rows[row]
		column := 0
		for col := 0; col+1 < len(cells); col++ {
			boundary := cells[col].End + (cells[col+1].Start-cells[col].End)/2
			for _, pipe := range line.Pipes {
				if pipe >= cells[col].End && pipe <= cells[col+1].Start {
					boundary = pipe
					break
				}
			}
			if offset > boundary || offset == boundary && affinity == PreferNextCell {
				column = col + 1
				continue
			}
			break
		}
		return Cell{Row: row, Col: column}, true
	}
	if offset == view.End && len(view.Rows) > 0 {
		row := len(view.Rows) - 1
		if width := len(view.Rows[row]); width > 0 {
			return Cell{Row: row, Col: width - 1}, true
		}
	}
	return Cell{}, false
}

// Matrix is a rectangular copy of cell text for clipboard and paste logic.
type Matrix struct {
	Cells      [][]string
	Alignments []Alignment
}

// Alignment is the semantic horizontal alignment of one table column.
type Alignment uint8

const (
	AlignmentNone Alignment = iota
	AlignmentLeft
	AlignmentCenter
	AlignmentRight
)

// CopyRegion reads a rectangular cell selection from the source view. Missing
// cells in ragged rows become empty strings, preserving the selected shape.
func CopyRegion(view DocumentView, region Selection) (Matrix, bool) {
	if view.LineStart < 0 || view.End < view.LineStart || view.End > len(view.Source) ||
		region.LineStart != view.LineStart || len(view.Rows) == 0 {
		return Matrix{}, false
	}
	top, left := min(region.Anchor.Row, region.Focus.Row), min(region.Anchor.Col, region.Focus.Col)
	bottom, right := max(region.Anchor.Row, region.Focus.Row), max(region.Anchor.Col, region.Focus.Col)
	if top < 0 || left < 0 || bottom >= len(view.Rows) {
		return Matrix{}, false
	}
	width := 0
	for _, row := range view.Rows {
		width = max(width, len(row))
	}
	if left >= width || right >= width {
		return Matrix{}, false
	}
	matrix := Matrix{
		Cells:      make([][]string, bottom-top+1),
		Alignments: make([]Alignment, right-left+1),
	}
	for col := left; col <= right && col < len(view.Alignments); col++ {
		matrix.Alignments[col-left] = view.Alignments[col]
	}
	for row := top; row <= bottom; row++ {
		matrix.Cells[row-top] = make([]string, right-left+1)
		for col := left; col <= right && col < len(view.Rows[row]); col++ {
			r := view.Rows[row][col]
			if r.Start < view.LineStart || r.End < r.Start || r.End > view.End || r.End > len(view.Source) {
				return Matrix{}, false
			}
			matrix.Cells[row-top][col-left] = view.Source[r.Start:r.End]
		}
	}
	return matrix, true
}

// Region converts two inclusive cell endpoints into the semantic table range
// used by selection, copy, and structural editing commands.
func (g Grid) Region(anchor, focus Cell, right bool) *Selection {
	if g.Rows <= 0 || g.Cols <= 0 {
		return nil
	}
	anchor.Row = clamp(anchor.Row, 0, g.Rows-1)
	anchor.Col = clamp(anchor.Col, 0, g.Cols-1)
	focus.Row = clamp(focus.Row, 0, g.Rows-1)
	focus.Col = clamp(focus.Col, 0, g.Cols-1)
	top, bottom := min(anchor.Row, focus.Row), max(anchor.Row, focus.Row)
	left, rightCol := min(anchor.Col, focus.Col), max(anchor.Col, focus.Col)
	axis, first, last := Cells, top, bottom
	switch {
	case top == 0 && left == 0 && bottom == g.Rows-1 && rightCol == g.Cols-1:
		axis, first, last = Whole, 0, g.Rows-1
	case left == 0 && rightCol == g.Cols-1:
		axis, first, last = Rows, top, bottom
	case top == 0 && bottom == g.Rows-1:
		axis, first, last = Columns, left, rightCol
	}
	legacyAnchor, active := anchor.Col, focus.Col
	if axis == Rows || axis == Whole {
		legacyAnchor, active = anchor.Row, focus.Row
	}
	return &Selection{
		LineStart: g.LineStart, Anchor: anchor, Focus: focus, CellRegion: true,
		Axis: axis, First: first, Last: last, LegacyAnchor: legacyAnchor,
		Active: active, ActiveRow: focus.Row, Right: right,
	}
}

func clamp(value, low, high int) int { return min(max(value, low), high) }

// MoveOrder returns the row or column permutation and post-removal insertion
// index for a drag gap measured in the original grid. Drops inside the moved
// selection, including its trailing edge, are no-ops.
func MoveOrder(count, first, last, gap int) (order []int, insertion int, ok bool) {
	if first < 0 || first > last || last >= count || gap < 0 || gap > count || gap >= first && gap <= last+1 {
		return nil, first, false
	}
	remaining := make([]int, 0, count-(last-first+1))
	for i := 0; i < count; i++ {
		if i < first || i > last {
			remaining = append(remaining, i)
		}
	}
	insertion = gap
	if gap > last {
		insertion -= last - first + 1
	}
	order = make([]int, 0, count)
	order = append(order, remaining[:insertion]...)
	for i := first; i <= last; i++ {
		order = append(order, i)
	}
	order = append(order, remaining[insertion:]...)
	return order, insertion, true
}

// Selection is an immutable value snapshot of a table's structural range.
type Selection struct {
	LineStart    int
	Anchor       Cell
	Focus        Cell
	CellRegion   bool
	Axis         Axis
	First        int
	Last         int
	LegacyAnchor int
	Active       int
	ActiveRow    int
	Right        bool
	Origin       SelectionOrigin
}

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

// ClearRegion clears selected cell contents while keeping table dimensions.
// The source ranges come from one read-only document view and all writes are
// committed as one undoable transaction by the host.
func (c *Controller) ClearRegion(host Host, region Selection) (bool, error) {
	view, ok := host.TableView(region.LineStart)
	if !ok {
		return false, nil
	}
	if _, ok := CopyRegion(view, region); !ok {
		return false, nil
	}
	top, left := min(region.Anchor.Row, region.Focus.Row), min(region.Anchor.Col, region.Focus.Col)
	bottom, right := max(region.Anchor.Row, region.Focus.Row), max(region.Anchor.Col, region.Focus.Col)
	var edits []editor.TextEdit
	for row := top; row <= bottom; row++ {
		for col := left; col <= right && col < len(view.Rows[row]); col++ {
			r := view.Rows[row][col]
			edits = append(edits, editor.TextEdit{Range: r, Text: ""})
		}
	}
	if len(edits) == 0 {
		return false, nil
	}
	caret := edits[0].Range.Start
	selection := host.TextSelection()
	err := host.Apply(editor.Transaction{
		Edits: edits, Before: selection,
		After: editor.Selection{Anchor: caret, Head: caret},
	})
	return err == nil, err
}

// ReplaceRegion overlays a matrix onto a selected cell rectangle, retaining
// cells outside the target and growing the table when needed. It serializes a
// single source replacement and delegates history/selection application to
// the host transaction boundary.
func (c *Controller) ReplaceRegion(host Host, region Selection, matrix Matrix) (bool, error) {
	view, ok := host.TableView(region.LineStart)
	if !ok || len(view.Rows) == 0 || len(matrix.Cells) == 0 || len(matrix.Cells[0]) == 0 {
		return false, nil
	}
	if region.LineStart != view.LineStart || region.Anchor.Row < 0 || region.Focus.Row < 0 || region.Anchor.Col < 0 || region.Focus.Col < 0 {
		return false, nil
	}
	top, left := min(region.Anchor.Row, region.Focus.Row), min(region.Anchor.Col, region.Focus.Col)
	bottom, right := max(region.Anchor.Row, region.Focus.Row), max(region.Anchor.Col, region.Focus.Col)
	if bottom >= len(view.Rows) {
		return false, nil
	}
	rowCount, colCount := len(view.Rows), 0
	for _, row := range view.Rows {
		colCount = max(colCount, len(row))
	}
	matrixWidth := 0
	for _, row := range matrix.Cells {
		matrixWidth = max(matrixWidth, len(row))
	}
	rowCount = max(rowCount, top+len(matrix.Cells))
	colCount = max(colCount, left+matrixWidth)
	values := make([][]string, rowCount)
	for row := range values {
		values[row] = make([]string, colCount)
		if row < len(view.Rows) {
			for col, cell := range view.Rows[row] {
				if cell.Start < view.LineStart || cell.End < cell.Start || cell.End > view.End || cell.End > len(view.Source) {
					return false, nil
				}
				values[row][col] = view.Source[cell.Start:cell.End]
			}
		}
	}
	for row := top; row <= bottom && row < len(values); row++ {
		for col := left; col <= right && col < len(values[row]); col++ {
			values[row][col] = ""
		}
	}
	for row, cells := range matrix.Cells {
		for col, value := range cells {
			values[top+row][left+col] = value
		}
	}
	alignments := make([]Alignment, colCount)
	copy(alignments, view.Alignments)
	for col, alignment := range matrix.Alignments {
		if left+col < len(alignments) {
			alignments[left+col] = alignment
		}
	}
	text := serialize(values, alignments, view.Prefix, view.ContinuationPrefix)
	selection := host.TextSelection()
	err := host.Apply(editor.Transaction{
		Edits:  []editor.TextEdit{{Range: editor.TextRange{Start: view.LineStart, End: view.End}, Text: text}},
		Before: selection,
		After:  editor.Selection{Anchor: view.LineStart, Head: view.LineStart},
	})
	return err == nil, err
}

// AddColumnRight inserts a blank cell after the final displayed column on
// each source row. Parser-provided pipe positions keep escaped pipes intact.
func (c *Controller) AddColumnRight(host Host, lineStart int) (bool, error) {
	view, ok := host.TableView(lineStart)
	if !ok || len(view.Lines) == 0 {
		return false, nil
	}
	selection := host.TextSelection()
	edits := make([]editor.TextEdit, 0, len(view.Lines))
	for index, line := range view.Lines {
		if len(line.Pipes) == 0 || line.Range.End < line.Range.Start || line.Range.End > len(view.Source) {
			continue
		}
		text := "   |"
		if index == 1 {
			text = " --- |"
		}
		lastPipe := line.Pipes[len(line.Pipes)-1]
		if lastPipe+1 < line.Range.End && strings.TrimSpace(view.Source[lastPipe+1:line.Range.End]) != "" {
			text = " |" + text
		}
		edits = append(edits, editor.TextEdit{Range: editor.TextRange{Start: line.Range.End, End: line.Range.End}, Text: text})
	}
	if len(edits) == 0 {
		return false, nil
	}
	tx := editor.Transaction{Edits: edits, Before: selection}
	tx.After = mapSelection(tx, selection)
	err := host.Apply(tx)
	return err == nil, err
}

// AddRowBelow appends a row with blank cells at the end of the table.
func (c *Controller) AddRowBelow(host Host, lineStart int) (bool, error) {
	view, ok := host.TableView(lineStart)
	if !ok || len(view.Lines) < 2 || len(view.Rows) == 0 {
		return false, nil
	}
	columns := 0
	for _, row := range view.Rows {
		columns = max(columns, len(row))
	}
	if columns == 0 {
		return false, nil
	}
	text := "\n" + view.ContinuationPrefix + "|" + strings.Repeat("   |", columns)
	selection := host.TextSelection()
	tx := editor.Transaction{
		Edits:  []editor.TextEdit{{Range: editor.TextRange{Start: view.End, End: view.End}, Text: text}},
		Before: selection,
	}
	tx.After = mapSelection(tx, selection)
	err := host.Apply(tx)
	return err == nil, err
}

// InsertRowAfter inserts a blank row after a displayed row and returns the
// caret offset in the inserted row. Row zero is the header; inserting after
// it places the new row below the delimiter.
func (c *Controller) InsertRowAfter(host Host, lineStart, row, column int) (bool, int, error) {
	view, ok := host.TableView(lineStart)
	if !ok || len(view.Lines) < 2 || len(view.Rows) == 0 || row < 0 || row >= len(view.Rows) {
		return false, 0, nil
	}
	columns := 0
	for _, cells := range view.Rows {
		columns = max(columns, len(cells))
	}
	if columns == 0 {
		return false, 0, nil
	}
	lineIndex := row + 1
	if lineIndex >= len(view.Lines) {
		return false, 0, nil
	}
	at := view.Lines[lineIndex].Range.End
	if row == 0 {
		at = view.Lines[1].Range.End
	}
	text := "\n" + view.ContinuationPrefix + "|" + strings.Repeat("   |", columns)
	column = clamp(column, 0, columns-1)
	caret := at + 1 + len(view.ContinuationPrefix) + 2 + column*4
	selection := host.TextSelection()
	err := host.Apply(editor.Transaction{
		Edits:  []editor.TextEdit{{Range: editor.TextRange{Start: at, End: at}, Text: text}},
		Before: selection,
		After:  editor.Selection{Anchor: caret, Head: caret},
	})
	return err == nil, caret, err
}

// InsertColumnBefore inserts a blank column before the displayed column,
// using parser-projected pipe offsets so escaped pipes remain untouched.
func (c *Controller) InsertColumnBefore(host Host, lineStart, column int) (bool, error) {
	view, ok := host.TableView(lineStart)
	if !ok || len(view.Lines) < 2 || column < 0 {
		return false, nil
	}
	columns := 0
	for _, row := range view.Rows {
		columns = max(columns, len(row))
	}
	if columns == 0 || column > columns {
		return false, nil
	}
	edits := make([]editor.TextEdit, 0, len(view.Lines))
	for lineIndex, line := range view.Lines {
		if len(line.Pipes) == 0 || line.Range.End < line.Range.Start || line.Range.End > len(view.Source) {
			continue
		}
		at := line.Range.End
		if column < len(line.Pipes)-1 {
			at = line.Pipes[column] + 1
		} else {
			at = line.Pipes[len(line.Pipes)-1] + 1
		}
		text := "   |"
		if lineIndex == 1 {
			text = " --- |"
		}
		edits = append(edits, editor.TextEdit{Range: editor.TextRange{Start: at, End: at}, Text: text})
	}
	if len(edits) == 0 {
		return false, nil
	}
	selection := host.TextSelection()
	tx := editor.Transaction{Edits: edits, Before: selection}
	tx.After = mapSelection(tx, selection)
	err := host.Apply(tx)
	return err == nil, err
}

// AlignColumn replaces one delimiter cell with the requested semantic
// alignment and returns the source edit mapping for host-owned selection
// state that is not part of the document selection.
func (c *Controller) AlignColumn(host Host, lineStart, column int, alignment Alignment) (bool, EditMapping, error) {
	view, ok := host.TableView(lineStart)
	if !ok || len(view.Lines) < 2 || column < 0 || len(view.Lines[1].Pipes) <= column+1 {
		return false, EditMapping{}, nil
	}
	spec := " --- "
	switch alignment {
	case AlignmentNone:
		spec = " --- "
	case AlignmentLeft:
		spec = " :-- "
	case AlignmentCenter:
		spec = " :-: "
	case AlignmentRight:
		spec = " --: "
	default:
		return false, EditMapping{}, nil
	}
	from, to := view.Lines[1].Pipes[column]+1, view.Lines[1].Pipes[column+1]
	if from < view.Lines[1].Range.Start || to < from || to > view.Lines[1].Range.End || to > len(view.Source) {
		return false, EditMapping{}, nil
	}
	if view.Source[from:to] == spec {
		return false, EditMapping{}, nil
	}
	selection := host.TextSelection()
	tx := editor.Transaction{
		Edits:  []editor.TextEdit{{Range: editor.TextRange{Start: from, End: to}, Text: spec}},
		Before: selection,
		After:  mapSelection(editor.Transaction{Edits: []editor.TextEdit{{Range: editor.TextRange{Start: from, End: to}, Text: spec}}}, selection),
	}
	err := host.Apply(tx)
	return err == nil, EditMapping{From: from, To: to, Inserted: len(spec)}, err
}

// EditMapping describes one replacement in source coordinates.
type EditMapping struct{ From, To, Inserted int }

func (m EditMapping) Offset(offset int) int {
	if offset >= m.To {
		return offset + m.Inserted - (m.To - m.From)
	}
	if offset > m.From {
		return m.From + m.Inserted
	}
	return offset
}

// Reorder moves a contiguous row or column selection to an original-grid gap
// and commits the serialized table as one editor transaction. It returns the
// insertion index after removing the selection.
func (c *Controller) Reorder(host Host, lineStart int, axis Axis, first, last, gap int) (bool, int, error) {
	view, ok := host.TableView(lineStart)
	if !ok || axis != Rows && axis != Columns {
		return false, first, nil
	}
	count := len(view.Rows)
	if axis == Columns {
		count = 0
		for _, row := range view.Rows {
			count = max(count, len(row))
		}
	}
	order, insertion, ok := MoveOrder(count, first, last, gap)
	if !ok {
		return false, insertion, nil
	}
	rows := make([][]string, len(view.Rows))
	for row, spans := range view.Rows {
		rows[row] = make([]string, len(spans))
		for col, span := range spans {
			if span.Start < view.LineStart || span.End < span.Start || span.End > view.End || span.End > len(view.Source) {
				return false, insertion, nil
			}
			rows[row][col] = view.Source[span.Start:span.End]
		}
	}
	alignments := append([]Alignment(nil), view.Alignments...)
	if axis == Rows {
		moved := make([][]string, len(order))
		for index, old := range order {
			moved[index] = rows[old]
		}
		rows = moved
	} else {
		movedAlignments := make([]Alignment, len(order))
		for index, old := range order {
			if old < len(alignments) {
				movedAlignments[index] = alignments[old]
			}
		}
		for row, values := range rows {
			moved := make([]string, len(order))
			for index, old := range order {
				if old < len(values) {
					moved[index] = values[old]
				}
			}
			rows[row] = moved
		}
		alignments = movedAlignments
	}
	text := serialize(rows, alignments, view.Prefix, view.ContinuationPrefix)
	selection := host.TextSelection()
	tx := editor.Transaction{
		Edits:  []editor.TextEdit{{Range: editor.TextRange{Start: view.LineStart, End: view.End}, Text: text}},
		Before: selection,
	}
	tx.After = mapSelection(tx, selection)
	err := host.Apply(tx)
	return err == nil, insertion, err
}

// DeleteRows removes a row range (or the full table) or clears that range for
// change mode. Row zero is the Markdown header; the delimiter remains fixed.
func (c *Controller) DeleteRows(host Host, lineStart, first, last int, whole, change bool) (bool, int, error) {
	view, ok := host.TableView(lineStart)
	if !ok || len(view.Lines) < 2 {
		return false, 0, nil
	}
	firstLine, lastLine := first, last
	if !whole {
		if first < 0 || last < first || last >= len(view.Rows) {
			return false, 0, nil
		}
		if first > 0 {
			firstLine++
		}
		if last > 0 {
			lastLine++
		}
	} else {
		firstLine, lastLine = 0, len(view.Lines)-1
	}
	if firstLine < 0 || lastLine >= len(view.Lines) || lastLine < firstLine {
		return false, 0, nil
	}
	a := view.Lines[firstLine].Range.Start
	z := view.Lines[lastLine].Range.End
	if !change && z < len(view.Source) {
		z++
	}
	replacement := ""
	if change {
		replacement = "\n"
	}
	selection := host.TextSelection()
	tx := editor.Transaction{
		Edits:  []editor.TextEdit{{Range: editor.TextRange{Start: a, End: z}, Text: replacement}},
		Before: selection,
		After:  editor.Selection{Anchor: a, Head: a},
	}
	if change {
		tx.After.Anchor++
		tx.After.Head++
	}
	err := host.Apply(tx)
	return err == nil, a, err
}

// DeleteTable removes the complete table block and returns its source
// insertion point. Prefix and continuation text are part of the projected
// line ranges supplied by the host.
func (c *Controller) DeleteTable(host Host, lineStart int) (bool, int, error) {
	return c.DeleteRows(host, lineStart, 0, 0, true, false)
}

// DeleteColumns deletes or clears an inclusive column range. It retains each
// row's source prefix and suffix while rebuilding separators and alignment.
func (c *Controller) DeleteColumns(host Host, lineStart, first, last int, change bool) (bool, int, error) {
	view, ok := host.TableView(lineStart)
	if !ok || len(view.Lines) < 2 || first < 0 || last < first {
		return false, 0, nil
	}
	columns := 0
	for _, row := range view.Rows {
		columns = max(columns, len(row))
	}
	if last >= columns {
		return false, 0, nil
	}
	var b strings.Builder
	for lineIndex, line := range view.Lines {
		if lineIndex > 0 {
			b.WriteByte('\n')
		}
		if len(line.Pipes) < 2 || line.Range.End < line.Range.Start || line.Range.End > len(view.Source) {
			return false, 0, nil
		}
		b.WriteString(view.Source[line.Range.Start : line.Pipes[0]+1])
		written := 0
		for col := 0; col < columns; col++ {
			selected := col >= first && col <= last
			if !change && selected {
				continue
			}
			if written > 0 {
				b.WriteString(" |")
			}
			value := ""
			if lineIndex == 1 {
				separator := "---"
				if col < len(view.Alignments) {
					switch view.Alignments[col] {
					case AlignmentLeft:
						separator = ":--"
					case AlignmentCenter:
						separator = ":-:"
					case AlignmentRight:
						separator = "--:"
					}
				}
				value = separator
			} else {
				rowIndex := lineIndex
				if lineIndex > 1 {
					rowIndex--
				}
				if rowIndex < len(view.Rows) && col < len(view.Rows[rowIndex]) {
					cell := view.Rows[rowIndex][col]
					if cell.Start < view.LineStart || cell.End < cell.Start || cell.End > line.Range.End || cell.End > len(view.Source) {
						return false, 0, nil
					}
					value = view.Source[cell.Start:cell.End]
				} else {
					value = ""
				}
				if change && selected {
					value = ""
				}
			}
			b.WriteByte(' ')
			b.WriteString(value)
			written++
		}
		if written > 0 {
			b.WriteString(" |")
		}
		lastPipe := line.Pipes[len(line.Pipes)-1]
		if lastPipe+1 < line.Range.End {
			b.WriteString(view.Source[lastPipe+1 : line.Range.End])
		}
	}
	start, end := view.Lines[0].Range.Start, view.Lines[len(view.Lines)-1].Range.End
	selection := host.TextSelection()
	tx := editor.Transaction{
		Edits:  []editor.TextEdit{{Range: editor.TextRange{Start: start, End: end}, Text: b.String()}},
		Before: selection,
		After:  editor.Selection{Anchor: start, Head: start},
	}
	err := host.Apply(tx)
	return err == nil, start, err
}

// TypeColumnSelection replaces the selected columns with the same text in the
// leftmost selected cell of each row, clearing the remaining selected cells.
func (c *Controller) TypeColumnSelection(host Host, lineStart, first, last int, text string) (bool, int, error) {
	view, ok := host.TableView(lineStart)
	if !ok || first < 0 || last < first {
		return false, 0, nil
	}
	var edits []editor.TextEdit
	for _, row := range view.Rows {
		for col := first; col <= last && col < len(row); col++ {
			value := ""
			if col == first {
				value = text
			}
			edits = append(edits, editor.TextEdit{Range: row[col], Text: value})
		}
	}
	if len(edits) == 0 {
		return false, 0, nil
	}
	caret := edits[0].Range.Start + len(text)
	transaction := editor.Transaction{
		Edits: edits, Before: host.TextSelection(),
		After: editor.Selection{Anchor: caret, Head: caret},
	}
	err := host.Apply(transaction)
	return err == nil, caret, err
}

// TypeBlock inserts text at the end of one cell column in every row.
func (c *Controller) TypeBlock(host Host, lineStart, column int, text string) (bool, int, error) {
	view, ok := host.TableView(lineStart)
	if !ok || column < 0 {
		return false, 0, nil
	}
	var edits []editor.TextEdit
	for _, row := range view.Rows {
		if column < len(row) {
			at := row[column].End
			edits = append(edits, editor.TextEdit{Range: editor.TextRange{Start: at, End: at}, Text: text})
		}
	}
	if len(edits) == 0 {
		return false, 0, nil
	}
	caret := edits[0].Range.Start + len(text)
	transaction := editor.Transaction{
		Edits: edits, Before: host.TextSelection(),
		After: editor.Selection{Anchor: caret, Head: caret},
	}
	err := host.Apply(transaction)
	return err == nil, caret, err
}

// BackspaceBlock removes the previously typed suffix from each cell in one
// column, but only where that suffix still exactly matches source.
func (c *Controller) BackspaceBlock(host Host, lineStart, column int, remove string) (bool, error) {
	view, ok := host.TableView(lineStart)
	if !ok || column < 0 || remove == "" {
		return false, nil
	}
	var edits []editor.TextEdit
	for _, row := range view.Rows {
		if column >= len(row) {
			continue
		}
		end := row[column].End
		start := end - len(remove)
		if start >= row[column].Start && end <= len(view.Source) && view.Source[start:end] == remove {
			edits = append(edits, editor.TextEdit{Range: editor.TextRange{Start: start, End: end}})
		}
	}
	if len(edits) == 0 {
		return false, nil
	}
	selection := host.TextSelection()
	transaction := editor.Transaction{Edits: edits, Before: selection}
	transaction.After = mapSelection(transaction, selection)
	err := host.Apply(transaction)
	return err == nil, err
}

// PasteRows inserts copied rows around a table row while keeping the first
// source row as the table header and leaving the delimiter row in place.
func (c *Controller) PasteRows(host Host, lineStart, row int, before bool, rows [][]string) (bool, int, error) {
	view, ok := host.TableView(lineStart)
	if !ok || len(view.Lines) < 2 || row < 0 || row >= len(view.Rows) || len(rows) == 0 {
		return false, 0, nil
	}
	insertedRow := row
	if row == 0 {
		insertedRow = 1
		if !before {
			insertedRow = len(rows)
		}
	} else {
		insertedRow += len(rows)
		insertedRow += len(rows)
	}
	at := view.Lines[len(view.Lines)-1].Range.End
	appendAfter := !before && row > 0
	if before && row > 0 {
		lineIndex := row + 1
		if lineIndex < len(view.Lines) {
			at = view.Lines[lineIndex].Range.Start
		}
	} else if !before && row > 0 && row+2 < len(view.Lines) {
		at = view.Lines[row+2].Range.Start
		appendAfter = false
	}
	if row == 0 {
		at = view.Lines[1].Range.End
		if at < len(view.Source) {
			at++
		}
		appendAfter = false
	}
	var b strings.Builder
	for index, values := range rows {
		if index > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(view.Prefix)
		b.WriteByte('|')
		for _, value := range values {
			b.WriteByte(' ')
			b.WriteString(value)
			b.WriteString(" |")
		}
	}
	text := b.String() + "\n"
	if appendAfter {
		text = "\n" + b.String()
	}
	selection := host.TextSelection()
	tx := editor.Transaction{
		Edits:  []editor.TextEdit{{Range: editor.TextRange{Start: at, End: at}, Text: text}},
		Before: selection,
		After:  editor.Selection{Anchor: at + len(text), Head: at + len(text)},
	}
	err := host.Apply(tx)
	return err == nil, insertedRow, err
}

// PasteColumns inserts a copied column matrix at a table boundary, expanding
// the table with rows when the copied matrix is taller than the destination.
func (c *Controller) PasteColumns(host Host, lineStart, row, column int, matrix Matrix) (bool, int, error) {
	view, ok := host.TableView(lineStart)
	if !ok || len(view.Lines) < 2 || row < 0 || row >= len(view.Rows) || len(matrix.Cells) == 0 || len(matrix.Cells[0]) == 0 || column < 0 {
		return false, 0, nil
	}
	columns := 0
	for _, values := range view.Rows {
		columns = max(columns, len(values))
	}
	if column > columns {
		return false, 0, nil
	}
	var b strings.Builder
	for lineIndex, line := range view.Lines {
		if lineIndex > 0 {
			b.WriteByte('\n')
		}
		if len(line.Pipes) < 2 || line.Range.End > len(view.Source) {
			b.WriteString(view.Source[line.Range.Start:line.Range.End])
			continue
		}
		b.WriteString(view.Source[line.Range.Start : line.Pipes[0]+1])
		written := 0
		for col := 0; col <= columns; col++ {
			if col == column {
				for matrixCol := range matrix.Cells[0] {
					if written > 0 {
						b.WriteString(" |")
					}
					value := ""
					if lineIndex == 1 {
						if matrixCol < len(matrix.Alignments) {
							switch matrix.Alignments[matrixCol] {
							case AlignmentLeft:
								value = " :-- "
							case AlignmentCenter:
								value = " :-: "
							case AlignmentRight:
								value = " --: "
							default:
								value = " --- "
							}
						} else {
							value = " --- "
						}
					} else if lineIndex == 0 && matrixCol < len(matrix.Cells[0]) {
						value = matrix.Cells[0][matrixCol]
					} else if lineIndex > 1 {
						matrixRow := lineIndex - 1
						if matrixRow < len(matrix.Cells) && matrixCol < len(matrix.Cells[matrixRow]) {
							value = matrix.Cells[matrixRow][matrixCol]
						}
					}
					b.WriteByte(' ')
					b.WriteString(value)
					written++
				}
			}
			if col == columns {
				break
			}
			if written > 0 {
				b.WriteString(" |")
			}
			value := ""
			if lineIndex == 1 {
				value = " --- "
			} else {
				matrixRow := lineIndex
				if lineIndex > 1 {
					matrixRow--
				}
				if matrixRow < len(view.Rows) && col < len(view.Rows[matrixRow]) {
					cell := view.Rows[matrixRow][col]
					if cell.Start < view.LineStart || cell.End < cell.Start || cell.End > line.Range.End || cell.End > len(view.Source) {
						return false, 0, nil
					}
					value = view.Source[cell.Start:cell.End]
				}
			}
			b.WriteByte(' ')
			b.WriteString(value)
			written++
		}
		if written > 0 {
			b.WriteString(" |")
		}
		lastPipe := line.Pipes[len(line.Pipes)-1]
		if lastPipe+1 < line.Range.End {
			b.WriteString(view.Source[lastPipe+1 : line.Range.End])
		}
	}
	for matrixRow := len(view.Rows); matrixRow < len(matrix.Cells); matrixRow++ {
		b.WriteByte('\n')
		b.WriteString(view.Prefix)
		b.WriteByte('|')
		written := 0
		for col := 0; col <= columns; col++ {
			if col == column {
				for matrixCol := range matrix.Cells[matrixRow] {
					if written > 0 {
						b.WriteString(" |")
					}
					b.WriteByte(' ')
					b.WriteString(matrix.Cells[matrixRow][matrixCol])
					written++
				}
			}
			if col == columns {
				break
			}
			if written > 0 {
				b.WriteString(" |")
			}
			b.WriteString("  ")
			written++
		}
		if written > 0 {
			b.WriteString(" |")
		}
	}
	start, end := view.Lines[0].Range.Start, view.Lines[len(view.Lines)-1].Range.End
	selection := host.TextSelection()
	tx := editor.Transaction{
		Edits:  []editor.TextEdit{{Range: editor.TextRange{Start: start, End: end}, Text: b.String()}},
		Before: selection,
		After:  editor.Selection{Anchor: start, Head: start},
	}
	err := host.Apply(tx)
	return err == nil, column, err
}

// PasteBlock inserts a copied table or cell fragment at a document offset,
// separating it from surrounding Markdown with the required blank lines.
func (c *Controller) PasteBlock(host Host, at int, fragment string) (bool, error) {
	source := host.DocumentSource()
	if at < 0 || at > len(source) || fragment == "" {
		return false, nil
	}
	if at > 0 {
		prefix := source[:at]
		fragment = strings.Repeat("\n", max(0, 2-(len(prefix)-len(strings.TrimRight(prefix, "\n"))))) + fragment
	}
	if at < len(source) {
		suffix := source[at:]
		fragment += strings.Repeat("\n", max(0, 2-(len(suffix)-len(strings.TrimLeft(suffix, "\n")))))
	}
	selection := host.TextSelection()
	caret := at + len(fragment)
	tx := editor.Transaction{
		Edits:  []editor.TextEdit{{Range: editor.TextRange{Start: at, End: at}, Text: fragment}},
		Before: selection,
		After:  editor.Selection{Anchor: caret, Head: caret},
	}
	err := host.Apply(tx)
	return err == nil, err
}

func mapSelection(transaction editor.Transaction, selection editor.Selection) editor.Selection {
	mapOffset := func(offset int) int {
		original := offset
		deltaTotal := 0
		for _, edit := range transaction.Edits {
			delta := len(edit.Text) - (edit.Range.End - edit.Range.Start)
			switch {
			case original >= edit.Range.End:
				deltaTotal += delta
			case original > edit.Range.Start:
				return edit.Range.Start + deltaTotal + len(edit.Text)
			default:
				return original + deltaTotal
			}
		}
		return original + deltaTotal
	}
	selection.Anchor = mapOffset(selection.Anchor)
	selection.Head = mapOffset(selection.Head)
	return selection
}

func serialize(rows [][]string, alignments []Alignment, prefix, continuationPrefix string) string {
	var b strings.Builder
	for row, cells := range rows {
		if row > 0 {
			b.WriteByte('\n')
		}
		if row == 0 {
			b.WriteString(prefix)
		} else {
			b.WriteString(continuationPrefix)
		}
		b.WriteByte('|')
		for _, value := range cells {
			b.WriteString(" " + value + " |")
		}
		if row == 0 {
			b.WriteByte('\n')
			b.WriteString(continuationPrefix)
			b.WriteByte('|')
			for col := range cells {
				separator := "---"
				switch alignments[col] {
				case AlignmentLeft:
					separator = ":---"
				case AlignmentCenter:
					separator = ":---:"
				case AlignmentRight:
					separator = "---:"
				}
				b.WriteString(" " + separator + " |")
			}
		}
	}
	return b.String()
}

func (c *Controller) ResetNavigation() { c.navigation = nil }

func (c *Controller) NavigationLen() int { return len(c.navigation) }

func (c *Controller) Replay(current Snapshot, rowDelta, colDelta int) (Snapshot, bool) {
	if len(c.navigation) == 0 {
		return Snapshot{}, false
	}
	step := c.navigation[len(c.navigation)-1]
	if !Equal(current, step.after) || rowDelta != -step.rowDelta || colDelta != -step.colDelta {
		if !Equal(current, step.after) {
			c.navigation = nil
		}
		return Snapshot{}, false
	}
	c.navigation = c.navigation[:len(c.navigation)-1]
	return Clone(step.before), true
}

func (c *Controller) Record(before, after Snapshot, rowDelta, colDelta int) {
	if Equal(before, after) {
		return
	}
	if before.Selection == nil && before.DocumentRegion == nil && before.Bridge == nil &&
		before.VirtualEdit == nil && before.Visual == nil &&
		after.Selection == nil && after.DocumentRegion == nil && after.Bridge == nil &&
		after.VirtualEdit == nil && after.Visual == nil &&
		len(c.navigation) == 0 {
		return
	}
	c.navigation = append(c.navigation, navigationStep{
		before: Clone(before), after: Clone(after), rowDelta: rowDelta, colDelta: colDelta,
	})
}

func Clone(s Snapshot) Snapshot {
	if s.Selection != nil {
		v := *s.Selection
		s.Selection = &v
	}
	if s.DocumentRegion != nil {
		v := *s.DocumentRegion
		s.DocumentRegion = &v
	}
	if s.Bridge != nil {
		v := *s.Bridge
		s.Bridge = &v
	}
	if s.VirtualEdit != nil {
		v := *s.VirtualEdit
		s.VirtualEdit = &v
	}
	if s.Visual != nil {
		v := *s.Visual
		s.Visual = &v
	}
	return s
}

func Equal(a, b Snapshot) bool {
	return a.Mode == b.Mode && a.TextSelection == b.TextSelection &&
		equalSelection(a.Selection, b.Selection) &&
		equalSelection(a.DocumentRegion, b.DocumentRegion) &&
		equalBridge(a.Bridge, b.Bridge) &&
		equalVirtualEdit(a.VirtualEdit, b.VirtualEdit) &&
		equalVisual(a.Visual, b.Visual)
}

func equalSelection(a, b *Selection) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalBridge(a, b *ContentBridge) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalVirtualEdit(a, b *VirtualEdit) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalVisual(a, b *VisualSelection) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
