// Package table owns the semantic state contract for block-table editing.
// The editor facade stores history snapshots of these values; view and source
// models are deliberately absent from this package.
package table

import "muse/editor"

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

// AxisRegion creates a row, column, or whole-table selection from its active
// endpoints. Endpoint orientation is retained in Anchor and Focus while the
// inclusive range remains normalized in First and Last.
func (g Grid) AxisRegion(axis Axis, anchor, focus int, right bool) *Selection {
	if g.Rows <= 0 || g.Cols <= 0 {
		return nil
	}
	switch axis {
	case Rows:
		selection := g.Region(Cell{Row: anchor}, Cell{Row: focus, Col: g.Cols - 1}, right)
		if selection != nil {
			selection.Axis = Rows
			selection.First, selection.Last = min(selection.Anchor.Row, selection.Focus.Row), max(selection.Anchor.Row, selection.Focus.Row)
			selection.LegacyAnchor, selection.Active, selection.ActiveRow = selection.Anchor.Row, selection.Focus.Row, selection.Focus.Row
		}
		return selection
	case Columns:
		selection := g.Region(Cell{Col: anchor}, Cell{Row: g.Rows - 1, Col: focus}, right)
		if selection != nil {
			selection.Axis = Columns
			selection.First, selection.Last = min(selection.Anchor.Col, selection.Focus.Col), max(selection.Anchor.Col, selection.Focus.Col)
			selection.LegacyAnchor, selection.Active = selection.Anchor.Col, selection.Focus.Col
		}
		return selection
	case Whole:
		return g.Region(Cell{}, Cell{Row: g.Rows - 1, Col: g.Cols - 1}, right)
	default:
		return nil
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
