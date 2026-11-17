package table

import (
	"strings"

	"muse/editor"
)

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
