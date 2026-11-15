// Package table owns the semantic state contract for block-table editing.
// The editor facade stores history snapshots of these values; view and source
// models are deliberately absent from this package.
package table

import (
	"strings"

	"muse/editor/internal/core"
)

// Host is the narrow bridge from table commands to the editor document. Views
// are immutable and short lived; writes are atomic editor transactions.
type Host interface {
	TableView(lineStart int) (DocumentView, bool)
	DocumentSource() string
	TextSelection() core.Selection
	Apply(core.Transaction) error
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
	Rows               [][]core.TextRange
	Lines              []LineView
	Alignments         []Alignment
}

// LineView contains a table source line and parser-computed unescaped pipe
// offsets, allowing commands to preserve whitespace and escaped delimiters.
type LineView struct {
	Range core.TextRange
	Pipes []int
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
	matrix := Matrix{Cells: make([][]string, bottom-top+1)}
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
	FromText     bool
	TextMode     string
}

type ContentBridge struct {
	LineStart       int
	Axis            Axis
	Row             int
	Col             int
	Direction       int
	Resume          string
	Index           int
	SelectionAnchor int
	FromText        bool
	TextMode        string
}

// Snapshot captures table-specific selection alongside the editor's text
// selection. It is the single value stored by editor undo/redo for table state.
type Snapshot struct {
	Mode           string
	TextSelection  core.Selection
	Selection      *Selection
	DocumentRegion *Selection
	Bridge         *ContentBridge
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
	var edits []core.TextEdit
	for row := top; row <= bottom; row++ {
		for col := left; col <= right && col < len(view.Rows[row]); col++ {
			r := view.Rows[row][col]
			edits = append(edits, core.TextEdit{Range: r, Text: ""})
		}
	}
	if len(edits) == 0 {
		return false, nil
	}
	caret := edits[0].Range.Start
	selection := host.TextSelection()
	err := host.Apply(core.Transaction{
		Edits: edits, Before: selection,
		After: core.Selection{Anchor: caret, Head: caret},
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
	err := host.Apply(core.Transaction{
		Edits:  []core.TextEdit{{Range: core.TextRange{Start: view.LineStart, End: view.End}, Text: text}},
		Before: selection,
		After:  core.Selection{Anchor: view.LineStart, Head: view.LineStart},
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
	edits := make([]core.TextEdit, 0, len(view.Lines))
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
		edits = append(edits, core.TextEdit{Range: core.TextRange{Start: line.Range.End, End: line.Range.End}, Text: text})
	}
	if len(edits) == 0 {
		return false, nil
	}
	tx := core.Transaction{Edits: edits, Before: selection}
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
	tx := core.Transaction{
		Edits:  []core.TextEdit{{Range: core.TextRange{Start: view.End, End: view.End}, Text: text}},
		Before: selection,
	}
	tx.After = mapSelection(tx, selection)
	err := host.Apply(tx)
	return err == nil, err
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
	tx := core.Transaction{
		Edits:  []core.TextEdit{{Range: core.TextRange{Start: view.LineStart, End: view.End}, Text: text}},
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
	tx := core.Transaction{
		Edits:  []core.TextEdit{{Range: core.TextRange{Start: a, End: z}, Text: replacement}},
		Before: selection,
		After:  core.Selection{Anchor: a, Head: a},
	}
	if change {
		tx.After.Anchor++
		tx.After.Head++
	}
	err := host.Apply(tx)
	return err == nil, a, err
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
	tx := core.Transaction{
		Edits:  []core.TextEdit{{Range: core.TextRange{Start: start, End: end}, Text: b.String()}},
		Before: selection,
		After:  core.Selection{Anchor: start, Head: start},
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
	var edits []core.TextEdit
	for _, row := range view.Rows {
		for col := first; col <= last && col < len(row); col++ {
			value := ""
			if col == first {
				value = text
			}
			edits = append(edits, core.TextEdit{Range: row[col], Text: value})
		}
	}
	if len(edits) == 0 {
		return false, 0, nil
	}
	caret := edits[0].Range.Start + len(text)
	transaction := core.Transaction{
		Edits: edits, Before: host.TextSelection(),
		After: core.Selection{Anchor: caret, Head: caret},
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
	var edits []core.TextEdit
	for _, row := range view.Rows {
		if column < len(row) {
			at := row[column].End
			edits = append(edits, core.TextEdit{Range: core.TextRange{Start: at, End: at}, Text: text})
		}
	}
	if len(edits) == 0 {
		return false, 0, nil
	}
	caret := edits[0].Range.Start + len(text)
	transaction := core.Transaction{
		Edits: edits, Before: host.TextSelection(),
		After: core.Selection{Anchor: caret, Head: caret},
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
	var edits []core.TextEdit
	for _, row := range view.Rows {
		if column >= len(row) {
			continue
		}
		end := row[column].End
		start := end - len(remove)
		if start >= row[column].Start && end <= len(view.Source) && view.Source[start:end] == remove {
			edits = append(edits, core.TextEdit{Range: core.TextRange{Start: start, End: end}})
		}
	}
	if len(edits) == 0 {
		return false, nil
	}
	selection := host.TextSelection()
	transaction := core.Transaction{Edits: edits, Before: selection}
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
	tx := core.Transaction{
		Edits:  []core.TextEdit{{Range: core.TextRange{Start: at, End: at}, Text: text}},
		Before: selection,
		After:  core.Selection{Anchor: at + len(text), Head: at + len(text)},
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
	tx := core.Transaction{
		Edits:  []core.TextEdit{{Range: core.TextRange{Start: start, End: end}, Text: b.String()}},
		Before: selection,
		After:  core.Selection{Anchor: start, Head: start},
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
	tx := core.Transaction{
		Edits:  []core.TextEdit{{Range: core.TextRange{Start: at, End: at}, Text: fragment}},
		Before: selection,
		After:  core.Selection{Anchor: caret, Head: caret},
	}
	err := host.Apply(tx)
	return err == nil, err
}

func mapSelection(transaction core.Transaction, selection core.Selection) core.Selection {
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
		after.Selection == nil && after.DocumentRegion == nil && after.Bridge == nil &&
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
	return s
}

func Equal(a, b Snapshot) bool {
	return a.Mode == b.Mode && a.TextSelection == b.TextSelection &&
		equalSelection(a.Selection, b.Selection) &&
		equalSelection(a.DocumentRegion, b.DocumentRegion) &&
		equalBridge(a.Bridge, b.Bridge)
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
