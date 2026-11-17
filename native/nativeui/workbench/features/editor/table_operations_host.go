package editor

import (
	"strings"
	"unicode/utf8"

	tablemodel "muse/editor/blocks/table"
	"muse/editor/input/vim"
	syntax "muse/markdown/syntax"
	tableui "muse/nativeui/workbench/features/editor/blocks/table"
)

func (e *Editor) selectedTableRows(t *leaf, s *tableSelection) []int {
	if t == nil || s == nil || s.lineStart != t.lineStart {
		return nil
	}
	if s.axis == tableRows {
		out := make([]int, 0, s.last-s.first+1)
		for i := s.first; i <= s.last; i++ {
			out = append(out, i)
		}
		return out
	}
	if s.axis == tableWhole {
		out := make([]int, len(t.tab.rows))
		for i := range out {
			out[i] = i
		}
		return out
	}
	return nil
}

func (e *Editor) tableRegisterForSelection(t *leaf, s *tableSelection) *tableRegister {
	if t == nil || s == nil || s.lineStart != t.lineStart {
		return nil
	}
	r := &tableRegister{axis: s.axis, whole: s.axis == tableWhole || s.axis == tableRows && s.first == 0 && s.last == len(t.tab.rows)-1 || s.axis == tableColumns && s.first == 0 && s.last == t.tab.cols()-1}
	if s.cellRegion {
		region, ok := e.tableSelectionCellRegion(s)
		if !ok {
			return nil
		}
		matrix := e.tableCellMatrix(t, region)
		if matrix == nil {
			return nil
		}
		top, _, _, _ := tableRegionBounds(region)
		return &tableRegister{axis: s.axis, whole: r.whole, rows: matrix.Cells, align: parserAlignmentsFromTable(matrix.Alignments), region: true, sourceHeader: top == 0}
	}
	if s.axis == tableRows || s.axis == tableWhole {
		r.align = append([]syntax.IncrementalAlign(nil), t.tab.align...)
		if s.first > 0 && s.axis != tableWhole {
			for _, cell := range t.tab.rows[0] {
				r.header = append(r.header, e.Source()[cell.start:cell.end])
			}
		}
		for _, rowIndex := range e.selectedTableRows(t, s) {
			row := t.tab.rows[rowIndex]
			values := make([]string, len(row))
			for i, cell := range row {
				values[i] = e.Source()[cell.start:cell.end]
			}
			r.rows = append(r.rows, values)
		}
		return r
	}
	if s.axis == tableColumns {
		for c := s.first; c <= s.last; c++ {
			r.align = append(r.align, alignAt(t.tab.align, c))
		}
		for _, row := range t.tab.rows {
			values := make([]string, 0, s.last-s.first+1)
			for c := s.first; c <= s.last; c++ {
				if c < len(row) {
					values = append(values, e.Source()[row[c].start:row[c].end])
				} else {
					values = append(values, "")
				}
			}
			r.rows = append(r.rows, values)
		}
		return r
	}
	return nil
}

func (r *tableRegister) markdown() string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	rows := r.rows
	if r.region && len(rows) > 0 {
		for i, row := range rows {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteByte('|')
			for _, cell := range row {
				b.WriteByte(' ')
				b.WriteString(cell)
				b.WriteString(" |")
			}
			if i == 0 {
				b.WriteString("\n|")
				for c := range row {
					sep := "---"
					switch alignAt(r.align, c) {
					case syntax.IncrementalAlignLeft:
						sep = ":---"
					case syntax.IncrementalAlignCenter:
						sep = ":---:"
					case syntax.IncrementalAlignRight:
						sep = "---:"
					}
					b.WriteString(" " + sep + " |")
				}
			}
		}
		return b.String()
	}
	if len(r.header) > 0 {
		rows = append([][]string{r.header}, rows...)
	}
	for i, row := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteByte('|')
		for _, cell := range row {
			b.WriteByte(' ')
			b.WriteString(cell)
			b.WriteString(" |")
		}
		if i == 0 {
			b.WriteString("\n|")
			for c := range row {
				sep := "---"
				switch alignAt(r.align, c) {
				case syntax.IncrementalAlignLeft:
					sep = ":---"
				case syntax.IncrementalAlignCenter:
					sep = ":---:"
				case syntax.IncrementalAlignRight:
					sep = "---:"
				}
				b.WriteString(" " + sep + " |")
			}
		}
	}
	return b.String()
}

func (r *tableRegister) html() string {
	if r == nil || !r.region || len(r.rows) == 0 {
		return ""
	}
	html := syntax.ParseIncrementalSource(r.markdown()).HTML()
	if !r.sourceHeader {
		html = strings.ReplaceAll(html, "<th>", "<td>")
		html = strings.ReplaceAll(html, "</th>", "</td>")
		html = strings.ReplaceAll(html, "<thead>", "<tbody>")
		html = strings.ReplaceAll(html, "</thead>", "</tbody>")
	}
	return html
}

// applyTableSelection performs a row or column edit in one undo step. The
// parser remains the authority for whether the result is still a GFM table.
func (e *Editor) applyTableSelection(op string) bool {
	s := e.tableSelection
	t := e.tableSelectionLeaf()
	if s == nil || t == nil {
		return false
	}
	if s.axis == tableCells {
		region, ok := e.tableSelectionCellRegion(s)
		if !ok {
			return false
		}
		top, left, bottom, right := tableRegionBounds(region)
		fullRows := left == 0 && right == t.tab.cols()-1
		fullColumns := top == 0 && bottom == len(t.tab.rows)-1
		switch {
		case fullRows && fullColumns:
			s.axis, s.first, s.last = tableWhole, 0, len(t.tab.rows)-1
		case fullRows:
			s.axis, s.first, s.last = tableRows, top, bottom
		case fullColumns:
			s.axis, s.first, s.last = tableColumns, left, right
		default:
			if op != "y" {
				e.setVimTableRegister(e.tableRegisterForSelection(t, s))
				if !e.clearTableCellRegion(t, region) {
					return false
				}
				if op == "c" {
					e.modalSetMode(vim.ModeInsert)
				} else {
					e.modalSetMode(vim.ModeNormal)
				}
				e.tableSelection = nil
				e.tableBridge = nil
				return true
			}
		}
	}
	reg := e.tableRegisterForSelection(t, s)
	e.setVimTableRegister(reg)
	if op == "y" {
		focus := e.selectionFocus()
		ranges := e.SelectionRanges()
		e.Copy()
		e.startVimYankFlash(ranges)
		// A yank doesn't leave a pending structural destination: the source
		// caret is still meaningful and p/P should resolve at its current cell.
		e.clearTableVirtualEdit()
		e.tableSelection = nil
		e.tableHandleDrag = nil
		e.tableBridge = nil
		e.resetTableNavigation()
		e.setSessionSelection(focus, focus)
		e.vimController().SetLinewise(false)
		e.setSessionInclusive(false)
		e.setSessionLinewise(false)
		if op == "d" {
			e.tableSelection, e.tableBridge, e.tableDocumentRegion = nil, nil, nil
			e.resetTableNavigation()
		}
		e.Session.UpdateLastHistoryAfter()
		return true
	}
	lines := e.tableLines(t)
	if len(lines) < 2 {
		return false
	}
	if s.axis == tableRows || s.axis == tableWhole {
		lineIndex := func(row int) int {
			if row == 0 {
				return 0
			}
			return row + 1
		}
		first, last := lineIndex(s.first), lineIndex(s.last)
		if s.axis == tableWhole {
			first, last = 0, len(lines)-1
		}
		if first >= len(lines) || last >= len(lines) {
			return false
		}
		virtualLineStart := t.lineStart
		virtualRow := s.first
		keepVirtual := op == "d" && s.axis == tableRows
		if s.axis == tableWhole {
			e.clearTableVirtualEdit()
		}
		result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.DeleteRows, LineStart: s.lineStart, First: s.first, Last: s.last, Whole: s.axis == tableWhole, Change: op == "c"})
		if err != nil || !result.Changed {
			return false
		}
		if keepVirtual {
			var updated *leaf
			for _, candidate := range e.leaves {
				if candidate.kind == kTable && candidate.tab != nil && candidate.lineStart == virtualLineStart {
					updated = candidate
					break
				}
			}
			if updated != nil && len(updated.tab.rows) > 0 {
				virtualRow = max(0, min(virtualRow, len(updated.tab.rows)-1))
				e.enterTableVirtualEdit(updated, virtualRow, 0, tableVirtualLeading, tableRows, true)
			} else {
				e.clearTableVirtualEdit()
			}
		}
		if op == "c" {
			e.modalSetMode(vim.ModeInsert)
		}
		e.vimController().SetLinewise(false)
		e.setSessionInclusive(false)
		e.setSessionLinewise(false)
		if op == "d" {
			e.tableSelection, e.tableBridge, e.tableDocumentRegion = nil, nil, nil
			e.resetTableNavigation()
		}
		e.Session.UpdateLastHistoryAfter()
		return true
	}
	if s.axis == tableColumns {
		virtualLineStart := t.lineStart
		virtualRow := max(0, min(s.activeRow, len(t.tab.rows)-1))
		virtualCol := s.first
		keepVirtual := op == "d"
		result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.DeleteColumns, LineStart: s.lineStart, First: s.first, Last: s.last, Change: op == "c"})
		if err != nil || !result.Changed {
			return false
		}
		if keepVirtual {
			var updated *leaf
			for _, candidate := range e.leaves {
				if candidate.kind == kTable && candidate.tab != nil && candidate.lineStart == virtualLineStart {
					updated = candidate
					break
				}
			}
			if updated != nil && updated.tab.cols() > 0 && len(updated.tab.rows) > 0 {
				virtualRow = max(0, min(virtualRow, len(updated.tab.rows)-1))
				virtualCol = max(0, min(virtualCol, updated.tab.cols()-1))
				e.enterTableVirtualEdit(updated, virtualRow, virtualCol, tableVirtualLeading, tableColumns, true)
			} else {
				e.clearTableVirtualEdit()
			}
		}
		if op == "c" {
			e.modalSetMode(vim.ModeInsert)
			e.modal.blockInsert = &tableBlockInsert{lineStart: s.lineStart, firstCol: s.first}
		}
		e.vimController().SetLinewise(false)
		e.setSessionInclusive(false)
		e.setSessionLinewise(false)
		return true
	}
	return false
}

func (e *Editor) typeTableColumnSelection(text string) bool {
	s := e.tableSelection
	t := e.tableSelectionLeaf()
	if s == nil || s.axis != tableColumns || t == nil {
		return false
	}
	result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.TypeColumnSelection, LineStart: s.lineStart, First: s.first, Last: s.last, Text: text})
	if err != nil || !result.Changed {
		return false
	}
	e.modalSetMode(vim.ModeInsert)
	e.modal.blockInsert = &tableBlockInsert{lineStart: s.lineStart, firstCol: s.first, text: text}
	return true
}

func (e *Editor) typeTableBlock(text string) bool {
	b := e.modal.blockInsert
	if b == nil {
		return false
	}
	result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.TypeBlock, LineStart: b.lineStart, Focus: tablemodel.Cell{Col: b.firstCol}, Text: text})
	if err != nil || !result.Changed {
		e.modal.blockInsert = nil
		return false
	}
	b.text += text
	return true
}

func (e *Editor) backspaceTableBlock() bool {
	b := e.modal.blockInsert
	if b == nil || b.text == "" {
		return b != nil
	}
	_, n := utf8.DecodeLastRuneInString(b.text)
	remove := b.text[len(b.text)-n:]
	result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.BackspaceBlock, LineStart: b.lineStart, Focus: tablemodel.Cell{Col: b.firstCol}, Text: remove})
	if err != nil || !result.Changed {
		e.modal.blockInsert = nil
		return false
	}
	b.text = b.text[:len(b.text)-n]
	return true
}

func (e *Editor) pasteTableRegister(before bool) bool {
	r := e.modal.registerTable
	if r == nil || len(r.rows) == 0 {
		return false
	}
	if s := e.tableSelection; s != nil && s.cellRegion {
		if t := e.tableSelectionLeaf(); t != nil {
			region, ok := e.tableSelectionCellRegion(s)
			rows := r.rows
			if len(r.header) > 0 {
				rows = append([][]string{r.header}, rows...)
			}
			matrix := &tableCellMatrix{Cells: rows, Alignments: tableAlignmentsFromParser(r.align)}
			if ok && e.replaceTableCellRegion(t, region, matrix) {
				e.tableSelection = nil
				e.tableBridge = nil
				e.tableDocumentRegion = nil
				e.Session.UpdateLastHistoryAfter()
				return true
			}
		}
	}
	// A structural delete/yank leaves its target as row/column coordinates.
	// Resolve that semantic position first: the text caret may be on a pipe or
	// separator where unitAt has no cell to return.
	var t *leaf
	row, col := -1, -1
	virtualLineStart := -1
	if r.region && r.axis == tableCells {
		t, row, col, _ = e.tableRegionAtCaret()
	}
	if t != nil {
		// Region registers use the focused cell as their semantic origin.
	} else if virtualTable, position, ok := e.resolveTableVirtualEdit(); ok {
		t, row, col = virtualTable, position.row, position.col
		virtualLineStart = position.lineStart
	} else {
		e.clearTableVirtualEdit()
		u := e.unitAt(e.Caret())
		if u != nil {
			t = e.tableOf(u)
			if t != nil {
				_, row, col = cellAt(t, e.Caret())
			}
		}
		if t == nil {
			if edge, after := e.tableAtEdge(e.Caret()); edge != nil {
				t = edge
				row, col = 0, 0
				if after && len(t.tab.rows) > 0 {
					row = len(t.tab.rows) - 1
					col = max(0, len(t.tab.rows[row])-1)
				}
			}
		}
		if t == nil {
			for _, candidate := range e.leaves {
				if candidate.kind != kTable || candidate.tab == nil || e.Caret() < candidate.lineStart || e.Caret() > blockEnd(e.Source(), candidate) {
					continue
				}
				t = candidate
				row, col, _ = e.tablePositionAtSource(t, e.Caret())
				break
			}
		} else if row < 0 || col < 0 {
			row, col, _ = e.tablePositionAtSource(t, e.Caret())
		}
	}
	if t != nil && virtualLineStart < 0 {
		virtualLineStart = t.lineStart
	}
	if t == nil || t.tab == nil || r.whole {
		value := r.markdown()
		if value == "" {
			return false
		}
		at, end := e.modalLine(e.selectionFocus())
		if !before {
			at = end
		}
		if t != nil {
			at = t.lineStart
			if !before {
				at = blockEnd(e.Source(), t)
			}
		}
		result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.PasteBlock, First: at, Text: value})
		if err != nil || !result.Changed {
			return false
		}
		e.clearTableVirtualEdit()
		return true
	}
	lines := e.tableLines(t)
	if len(lines) < 2 || row < 0 || col < 0 {
		return false
	}
	if r.region && r.axis == tableCells {
		startCol := col
		if !before {
			startCol++
		}
		region := tablemodel.Selection{
			LineStart: t.lineStart, Anchor: tablemodel.Cell{Row: row, Col: startCol},
			Focus: tablemodel.Cell{Row: row, Col: startCol}, CellRegion: true,
		}
		if e.replaceTableCellRegion(t, region, &tableCellMatrix{Cells: r.rows, Alignments: tableAlignmentsFromParser(r.align)}) {
			e.clearTableVirtualEdit()
			return true
		}
		return false
	}
	if r.axis == tableRows {
		result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.PasteRows, LineStart: t.lineStart, First: row, Before: before, Matrix: tablemodel.Matrix{Cells: r.rows}})
		if err != nil || !result.Changed {
			return false
		}
		insertedRow := result.Insertion
		var updated *leaf
		for _, candidate := range e.leaves {
			if candidate.kind == kTable && candidate.tab != nil && candidate.lineStart == t.lineStart {
				updated = candidate
				break
			}
		}
		if updated != nil && len(updated.tab.rows) > 0 {
			insertedRow = max(0, min(insertedRow, len(updated.tab.rows)-1))
			e.enterTableVirtualEdit(updated, insertedRow, 0, tableVirtualLeading, tableRows, false)
		} else {
			e.clearTableVirtualEdit()
		}
		e.tableSelection, e.tableBridge, e.tableDocumentRegion = nil, nil, nil
		e.Session.UpdateLastHistoryAfter()
		return true
	}
	if r.axis == tableColumns {
		virtualRow := row
		col = max(0, min(col, t.tab.cols()-1))
		if !before {
			col++
		}
		matrix := tablemodel.Matrix{Cells: r.rows, Alignments: tableAlignmentsFromParser(r.align)}
		result, err := e.tableUI().Dispatch(tableui.Action{Kind: tableui.PasteColumns, LineStart: virtualLineStart, First: virtualRow, Focus: tablemodel.Cell{Col: col}, Matrix: matrix})
		if err != nil || !result.Changed {
			return false
		}
		var updated *leaf
		for _, candidate := range e.leaves {
			if candidate.kind == kTable && candidate.tab != nil && candidate.lineStart == virtualLineStart {
				updated = candidate
				break
			}
		}
		if updated != nil && len(updated.tab.rows) > 0 {
			virtualRow = max(0, min(virtualRow, len(updated.tab.rows)-1))
			insertedCol := max(0, min(col, updated.tab.cols()-1))
			if insertedCol < len(updated.tab.rows[virtualRow]) {
				e.enterTableVirtualEdit(updated, virtualRow, insertedCol, tableVirtualLeading, tableColumns, false)
			} else {
				e.clearTableVirtualEdit()
			}
		} else {
			e.clearTableVirtualEdit()
		}
		e.tableSelection, e.tableBridge, e.tableDocumentRegion = nil, nil, nil
		e.Session.UpdateLastHistoryAfter()
		return true
	}
	return false
}
