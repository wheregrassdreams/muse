package table

import "muse/editor"

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
