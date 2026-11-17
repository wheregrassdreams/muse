package table

import "muse/editor"

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

// Bounds returns the inclusive rectangular cell bounds represented by a
// structural selection, independent of its anchor/focus orientation.
func (s Selection) Bounds() (top, left, bottom, right int) {
	top, bottom = s.Anchor.Row, s.Focus.Row
	left, right = s.Anchor.Col, s.Focus.Col
	if top > bottom {
		top, bottom = bottom, top
	}
	if left > right {
		left, right = right, left
	}
	return
}

// Contains reports whether a cell is part of this selection's semantic axis
// or rectangular cell region.
func (s Selection) Contains(cell Cell) bool {
	switch s.Axis {
	case Rows:
		return cell.Row >= s.First && cell.Row <= s.Last
	case Columns:
		return cell.Col >= s.First && cell.Col <= s.Last
	case Whole:
		return true
	case Cells:
		top, left, bottom, right := s.Bounds()
		return cell.Row >= top && cell.Row <= bottom && cell.Col >= left && cell.Col <= right
	default:
		return false
	}
}

// SourceRange projects a structural selection to the smallest source envelope
// that covers its selected table rows, columns, cells, or whole table.
func (s Selection) SourceRange(view DocumentView) (editor.TextRange, bool) {
	if !validView(view) || s.LineStart != view.LineStart || len(view.Rows) == 0 {
		return editor.TextRange{}, false
	}
	switch s.Axis {
	case Whole:
		return editor.TextRange{Start: view.LineStart, End: view.End}, true
	case Rows:
		first, last := clamp(s.First, 0, len(view.Rows)-1), clamp(s.Last, 0, len(view.Rows)-1)
		if first > last {
			first, last = last, first
		}
		firstLine, lastLine := tableLineForRow(first), tableLineForRow(last)
		if firstLine >= len(view.Lines) || lastLine >= len(view.Lines) {
			return editor.TextRange{}, false
		}
		return editor.TextRange{Start: view.Lines[firstLine].Range.Start, End: view.Lines[lastLine].Range.End}, true
	case Columns:
		if s.First <= 0 && s.Last >= maxRowWidth(view.Rows)-1 {
			return editor.TextRange{Start: view.LineStart, End: view.End}, true
		}
		if len(view.Rows[0]) == 0 || len(view.Rows[len(view.Rows)-1]) == 0 {
			return editor.TextRange{}, false
		}
		first, last := max(0, s.First), max(0, s.Last)
		if first > last {
			first, last = last, first
		}
		startCell := view.Rows[0][min(first, len(view.Rows[0])-1)]
		endRow := view.Rows[len(view.Rows)-1]
		endCell := endRow[min(last, len(endRow)-1)]
		return checkedRange(view, startCell.Start, endCell.End)
	case Cells:
		top, left, bottom, right := s.Bounds()
		if top < 0 || bottom >= len(view.Rows) || len(view.Rows[top]) == 0 || len(view.Rows[bottom]) == 0 {
			return editor.TextRange{}, false
		}
		startCell := view.Rows[top][min(left, len(view.Rows[top])-1)]
		endCell := view.Rows[bottom][min(right, len(view.Rows[bottom])-1)]
		return checkedRange(view, startCell.Start, endCell.End)
	default:
		return editor.TextRange{}, false
	}
}

// ContentRange projects the selected table content without the surrounding
// row pipes. It is used when structural navigation transitions into text.
func (s Selection) ContentRange(view DocumentView) (editor.TextRange, bool) {
	if !validView(view) || s.LineStart != view.LineStart || len(view.Rows) == 0 {
		return editor.TextRange{}, false
	}
	if s.Axis == Rows {
		first, last := clamp(s.First, 0, len(view.Rows)-1), clamp(s.Last, 0, len(view.Rows)-1)
		if first > last {
			first, last = last, first
		}
		firstRow, lastRow := view.Rows[first], view.Rows[last]
		if len(firstRow) == 0 || len(lastRow) == 0 {
			return editor.TextRange{}, false
		}
		return checkedRange(view, firstRow[0].Start, lastRow[len(lastRow)-1].End)
	}
	if s.Axis == Columns {
		if len(view.Rows[0]) == 0 || len(view.Rows[len(view.Rows)-1]) == 0 {
			return editor.TextRange{}, false
		}
		first, last := max(0, s.First), max(0, s.Last)
		if first > last {
			first, last = last, first
		}
		firstCell := view.Rows[0][min(first, len(view.Rows[0])-1)]
		lastRow := view.Rows[len(view.Rows)-1]
		lastCell := lastRow[min(last, len(lastRow)-1)]
		return checkedRange(view, firstCell.Start, lastCell.End)
	}
	if s.Axis == Cells {
		top, left, bottom, right := s.Bounds()
		if top < 0 || bottom >= len(view.Rows) || len(view.Rows[top]) == 0 || len(view.Rows[bottom]) == 0 {
			return editor.TextRange{}, false
		}
		startCell := view.Rows[top][min(left, len(view.Rows[top])-1)]
		endCell := view.Rows[bottom][min(right, len(view.Rows[bottom])-1)]
		return checkedRange(view, startCell.Start, endCell.End)
	}
	return s.SourceRange(view)
}

func tableLineForRow(row int) int {
	if row == 0 {
		return 0
	}
	return row + 1
}

func checkedRange(view DocumentView, start, end int) (editor.TextRange, bool) {
	if start < view.LineStart || end < start || end > view.End || end > len(view.Source) {
		return editor.TextRange{}, false
	}
	return editor.TextRange{Start: start, End: end}, true
}
