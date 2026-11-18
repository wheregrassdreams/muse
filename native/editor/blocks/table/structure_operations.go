package table

import (
	"strings"

	mdedit "github.com/wheregrassdreams/muse-editor/markdown/edit"
	"github.com/wheregrassdreams/muse-editor/text"
	"muse/editor"
)

// AddColumnRight inserts a blank cell after the final displayed column on
// each source row. Parser-provided pipe positions keep escaped pipes intact.
func (c *Controller) AddColumnRight(host Host, lineStart int) (bool, error) {
	view, ok := host.TableView(lineStart)
	if !ok || len(view.Lines) == 0 {
		return false, nil
	}
	at, ok := tableContentAnchor(view)
	if !ok {
		return false, nil
	}
	// Give the parser a line boundary at EOF so its table range includes a
	// final row that has no trailing newline.
	plannerSource := view.Source
	if !strings.HasSuffix(plannerSource, "\n") {
		if strings.HasSuffix(plannerSource, "\r") || strings.Contains(plannerSource, "\r\n") {
			plannerSource += "\r\n"
		} else {
			plannerSource += "\n"
		}
	}
	snapshot := mdedit.ParseSource(plannerSource)
	table, ok := snapshot.TableAt(at)
	if !ok {
		return false, nil
	}
	plan, ok := snapshot.InsertTableColumn(at, len(table.Lines[0].Cells))
	if !ok || !minimizeTableLineEdits(view, &plan) {
		return false, nil
	}
	tx := editor.TransactionFromPlan(plan, host.TextSelection())
	err := host.Apply(tx)
	return err == nil, err
}

// AddRowBelow appends a row with blank cells at the end of the table.
func (c *Controller) AddRowBelow(host Host, lineStart int) (bool, error) {
	view, ok := host.TableView(lineStart)
	if !ok || len(view.Lines) < 2 || len(view.Rows) == 0 {
		return false, nil
	}
	at, ok := tableContentAnchor(view)
	if !ok {
		return false, nil
	}
	snapshot := mdedit.ParseSource(view.Source)
	table, ok := snapshot.TableAt(at)
	if !ok || len(table.Lines) < 2 {
		return false, nil
	}
	plan, ok := snapshot.InsertTableRow(at, len(table.Lines)-1)
	if !ok || len(plan.Edits) != 1 {
		return false, nil
	}
	lastLine := view.Lines[len(view.Lines)-1].Range
	edit := &plan.Edits[0]
	oldEnd := int(edit.Range.To)
	oldStart := int(edit.Range.From)
	if oldEnd < oldStart || oldEnd-oldStart > 2 || oldStart > lastLine.End {
		return false, nil
	}
	// The syntax projection may stop at the last text node (before trailing
	// cell padding and pipe). Reanchor the planner's final-row insertion to the
	// host's complete source line, preserving a real line ending if it consumed
	// one.
	edit.Range = text.Range{From: text.Offset(lastLine.End), To: text.Offset(lastLine.End + oldEnd - oldStart)}
	if view.ContinuationPrefix != "" {
		rowStart := 0
		if strings.HasPrefix(edit.Insert, "\r\n") {
			rowStart = 2
		} else if strings.HasPrefix(edit.Insert, "\n") {
			rowStart = 1
		}
		row := edit.Insert[rowStart:]
		if strings.HasPrefix(row, table.Prefix) {
			row = view.ContinuationPrefix + row[len(table.Prefix):]
		} else {
			row = view.ContinuationPrefix + row
		}
		edit.Insert = edit.Insert[:rowStart] + row
	}
	selection := text.Cursor(text.Offset(int(edit.Range.From) + len(edit.Insert)))
	plan.Selection = &selection
	tx := editor.TransactionFromPlan(plan, host.TextSelection())
	err := host.Apply(tx)
	return err == nil, err
}

// tableContentAnchor is a source position within a parsed table cell. The
// Markdown AST's table range begins at its first text node, which may be after
// the leading pipe and cell padding, so the table's first line offset is not
// necessarily a valid lookup position.
func tableContentAnchor(view DocumentView) (text.Offset, bool) {
	for _, row := range view.Rows {
		for _, cell := range row {
			if cell.Start < cell.End {
				return text.Offset(cell.Start), true
			}
		}
	}
	// AddColumnRight only needs parser-projected line/pipe positions. Some
	// callers do not need the rendered cell ranges and omit Rows entirely.
	for lineIndex, line := range view.Lines {
		if lineIndex == 1 { // skip the GFM delimiter row when possible
			continue
		}
		if at, ok := contentInTableLine(view.Source, line); ok {
			return text.Offset(at), true
		}
	}
	if len(view.Lines) > 1 {
		if at, ok := contentInTableLine(view.Source, view.Lines[1]); ok {
			return text.Offset(at), true
		}
	}
	return 0, false
}

func contentInTableLine(source string, line LineView) (int, bool) {
	if line.Range.Start < 0 || line.Range.End < line.Range.Start || line.Range.End > len(source) {
		return 0, false
	}
	start := line.Range.Start
	for _, pipe := range line.Pipes {
		if pipe < start || pipe >= line.Range.End {
			return 0, false
		}
		if at, ok := firstTableContent(source, start, pipe); ok {
			return at, true
		}
		start = pipe + 1
	}
	return firstTableContent(source, start, line.Range.End)
}

func firstTableContent(source string, start, end int) (int, bool) {
	for start < end {
		switch source[start] {
		case ' ', '\t', '>':
			start++
		default:
			return start, true
		}
	}
	return 0, false
}

func minimizeTableLineEdits(view DocumentView, plan *mdedit.Plan) bool {
	if plan == nil || len(plan.Edits) != len(view.Lines) {
		return false
	}
	edits := make([]text.Edit, len(plan.Edits))
	for i, edit := range plan.Edits {
		line := view.Lines[i].Range
		if line.Start < 0 || line.End < line.Start || line.End > len(view.Source) ||
			int(edit.Range.From) != line.Start || int(edit.Range.To) != line.End {
			return false
		}
		original := view.Source[line.Start:line.End]
		prefix := 0
		for prefix < len(original) && prefix < len(edit.Insert) && original[prefix] == edit.Insert[prefix] {
			prefix++
		}
		suffix := 0
		for suffix < len(original)-prefix && suffix < len(edit.Insert)-prefix &&
			original[len(original)-1-suffix] == edit.Insert[len(edit.Insert)-1-suffix] {
			suffix++
		}
		edits[i] = text.Edit{
			Range:  text.Range{From: text.Offset(line.Start + prefix), To: text.Offset(line.End - suffix)},
			Insert: edit.Insert[prefix : len(edit.Insert)-suffix],
		}
	}
	plan.Edits = edits
	return true
}

// InsertRowAfter inserts a blank row after a displayed row and returns the
// caret offset in the inserted row. Row zero is the header; inserting after
// it places the new row below the delimiter.
func (c *Controller) InsertRowAfter(host Host, lineStart, row, column int) (bool, int, error) {
	view, ok := host.TableView(lineStart)
	if !ok || len(view.Lines) < 2 || len(view.Rows) == 0 || row < 0 || row >= len(view.Rows) {
		return false, 0, nil
	}
	snapshot := mdedit.ParseSource(view.Source)
	table, ok := snapshot.TableAt(text.Offset(view.LineStart))
	if !ok || len(table.Lines) < 2 || row >= len(table.Lines)-1 {
		return false, 0, nil
	}
	columns := len(table.Lines[0].Cells)
	if columns == 0 {
		return false, 0, nil
	}
	sourceRow := row
	if row > 0 {
		sourceRow++ // the GFM delimiter separates the displayed rows
	}
	plan, ok := snapshot.InsertTableRow(text.Offset(view.LineStart), sourceRow)
	if !ok || len(plan.Edits) != 1 {
		return false, 0, nil
	}
	column = clamp(column, 0, columns-1)
	edit := plan.Edits[0]
	rowStart := int(edit.Range.From)
	if strings.HasPrefix(edit.Insert, "\r\n") {
		rowStart += 2
	} else if strings.HasPrefix(edit.Insert, "\n") {
		rowStart++
	}
	caret := rowStart + len(table.Prefix) + 2 + column*4
	selection := text.Cursor(text.Offset(caret))
	plan.Selection = &selection
	tx := editor.TransactionFromPlan(plan, host.TextSelection())
	err := host.Apply(tx)
	return err == nil, caret, err
}

// InsertColumnBefore inserts a blank column before the displayed column,
// using parser-projected pipe offsets so escaped pipes remain untouched.
func (c *Controller) InsertColumnBefore(host Host, lineStart, column int) (bool, error) {
	view, ok := host.TableView(lineStart)
	if !ok || len(view.Lines) < 2 || column < 0 {
		return false, nil
	}
	plan, ok := mdedit.ParseSource(view.Source).InsertTableColumn(text.Offset(view.LineStart), column)
	if !ok {
		return false, nil
	}
	tx := editor.TransactionFromPlan(plan, host.TextSelection())
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
	name := "none"
	switch alignment {
	case AlignmentLeft:
		name = "left"
	case AlignmentCenter:
		name = "center"
	case AlignmentRight:
		name = "right"
	case AlignmentNone:
	default:
		return false, EditMapping{}, nil
	}
	plan, ok := mdedit.ParseSource(view.Source).AlignTableColumn(text.Offset(view.LineStart), column, name)
	if !ok || len(plan.Edits) != 1 {
		return false, EditMapping{}, nil
	}
	edit := plan.Edits[0]
	if alignment == AlignmentLeft {
		edit.Insert = " :-- "
	} else if alignment == AlignmentCenter {
		edit.Insert = " :-: "
	} else if alignment == AlignmentRight {
		edit.Insert = " --: "
	}
	from, to := int(edit.Range.From), int(edit.Range.To)
	if view.Source[from:to] == edit.Insert {
		return false, EditMapping{}, nil
	}
	tx := editor.TransactionFromPlan(mdedit.Plan{Edits: []text.Edit{edit}}, host.TextSelection())
	err := host.Apply(tx)
	return err == nil, EditMapping{From: from, To: to, Inserted: len(edit.Insert)}, err
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
	tx.After = tx.MapSelection(selection)
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
