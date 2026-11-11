package mdedit

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
)

type tableAxis uint8

const (
	tableRows tableAxis = iota + 1
	tableColumns
	tableWhole
)

// tableSelection is a semantic selection. first/last are inclusive row or
// column indexes; a table selection uses the whole table.
type tableSelection struct {
	lineStart int
	axis      tableAxis
	first     int
	last      int
	anchor    int
	active    int
	activeRow int
	right     bool
	fromText  bool
	textMode  string
}

type tableSelectDrag struct {
	lineStart int
	axis      tableAxis
	anchor    int
	shift     bool
}

type tableContentBridge struct {
	lineStart       int
	axis            tableAxis
	row             int
	col             int
	dir             int
	resume          string
	index           int
	selectionAnchor int
	fromText        bool
	textMode        string
}

// tableNavigationState keeps text endpoints separate from the active structure.
// A bridge owns its current text focus; structure ranges are always derived.
type tableNavigationState struct {
	mode          string
	anchor, caret int
	structure     *tableSelection
	bridge        *tableContentBridge
}

type tableNavigationStep struct {
	before, after tableNavigationState
	dir           int
}

func (e *Editor) tableNavigationState() tableNavigationState {
	s := tableNavigationState{mode: e.modal.mode, anchor: e.anchor, caret: e.caret}
	if e.tableSelection != nil {
		v := *e.tableSelection
		s.structure = &v
	}
	if e.tableBridge != nil {
		v := *e.tableBridge
		s.bridge = &v
	}
	return s
}

func sameTableNavigationState(a, b tableNavigationState) bool {
	if a.mode != b.mode || a.anchor != b.anchor || a.caret != b.caret || (a.structure == nil) != (b.structure == nil) || (a.bridge == nil) != (b.bridge == nil) {
		return false
	}
	return (a.structure == nil || *a.structure == *b.structure) && (a.bridge == nil || *a.bridge == *b.bridge)
}

func (e *Editor) restoreTableNavigation(s tableNavigationState) {
	e.modal.mode = s.mode
	e.anchor, e.caret = s.anchor, s.caret
	e.tableSelection, e.tableBridge = nil, nil
	if s.structure != nil {
		v := *s.structure
		e.tableSelection = &v
	}
	if s.bridge != nil {
		v := *s.bridge
		e.tableBridge = &v
	}
	e.hasGoal = false
	e.wantReveal = true
	e.blinkStart = time.Now()
	e.invalidate()
}

// selectionFocus is the active variant's focus, never the dormant text head.
func (e *Editor) selectionFocus() int {
	s, t := e.tableSelection, e.tableSelectionLeaf()
	if s != nil && t != nil {
		if s.axis == tableWhole {
			if s.right {
				return blockEnd(e.src, t)
			}
			return t.lineStart
		}
		if s.axis == tableRows {
			return e.tableRowBoundary(t, s.active, s.right)
		}
		if s.axis == tableColumns {
			row := t.tab.rows[max(0, min(s.activeRow, len(t.tab.rows)-1))]
			cell := row[max(0, min(s.active, len(row)-1))]
			if s.right {
				return cell.end
			}
			return cell.start
		}
	}
	return e.caret
}

type tableRegister struct {
	axis   tableAxis
	rows   [][]string
	align  []parser.Align
	header []string // source header when the selected rows exclude it
	whole  bool
}

type tableBlockInsert struct {
	lineStart int
	firstCol  int
	rows      []int
	text      string
}

func (e *Editor) tableSelectionLeaf() *leaf {
	if e.tableSelection == nil {
		return nil
	}
	for _, l := range e.leaves {
		if l.kind == kTable && l.lineStart == e.tableSelection.lineStart && l.tab != nil {
			return l
		}
	}
	return nil
}

func (e *Editor) beginTableSelectionDrag(t *leaf, axis tableAxis, index int, shift bool) {
	if t == nil {
		return
	}
	anchor := index
	if shift && e.tableSelection != nil && e.tableSelection.lineStart == t.lineStart && e.tableSelection.axis == axis {
		anchor = e.tableSelection.anchor
	}
	e.tableSelectDrag = &tableSelectDrag{lineStart: t.lineStart, axis: axis, anchor: anchor, shift: shift}
	e.selectTableAxis(t, axis, anchor, index, false)
}

func (e *Editor) updateTableSelectionDrag(index int) {
	d := e.tableSelectDrag
	if d == nil {
		return
	}
	t := e.tableSelectionLeaf()
	if t == nil || t.lineStart != d.lineStart {
		e.tableSelectDrag = nil
		return
	}
	e.selectTableAxis(t, d.axis, d.anchor, index, false)
}

func (e *Editor) selectTableAxis(t *leaf, axis tableAxis, first, last int, extend bool) {
	if !e.tableNavigating {
		e.tableNavigation = nil
	}
	if t == nil || t.tab == nil {
		return
	}
	limit := len(t.tab.rows)
	if axis == tableColumns {
		limit = t.tab.cols()
	} else if axis == tableWhole {
		first, last = 0, max(limit-1, 0)
	}
	if limit == 0 {
		return
	}
	anchor := max(0, min(first, limit-1))
	active := max(0, min(last, limit-1))
	first, last = max(0, min(first, limit-1)), max(0, min(last, limit-1))
	if first > last {
		first, last = last, first
	}
	if extend && e.tableSelection != nil && e.tableSelection.lineStart == t.lineStart && e.tableSelection.axis == axis {
		anchor = e.tableSelection.anchor
		last = max(0, min(last, limit-1))
		first = min(anchor, last)
		last = max(anchor, last)
	}
	e.tableSelection = &tableSelection{lineStart: t.lineStart, axis: axis, first: first, last: last, anchor: anchor, active: active, activeRow: 0, right: true, textMode: e.modal.mode}
	e.tableBridge = nil
	if axis == tableRows {
		e.tableSelection.activeRow = active
	}
	e.compose = ""
	e.hasGoal = false
	e.wantReveal = true
	e.blinkStart = time.Now()
	e.invalidate()
}

// tableHorizontalTransition exposes adjacent cell text first. Continuing to
// the far edge promotes the traversed row or column to a structure.
func (e *Editor) tableHorizontalTransition(dir int) bool {
	if b := e.tableBridge; b != nil {
		if dir == b.dir {
			var t *leaf
			for _, l := range e.leaves {
				if l.kind == kTable && l.lineStart == b.lineStart {
					t = l
					break
				}
			}
			if t != nil && b.row < len(t.tab.rows) && b.col < len(t.tab.rows[b.row]) {
				cell := t.tab.rows[b.row][b.col]
				atEdge := dir > 0 && e.caret == cell.end || dir < 0 && e.caret == cell.start
				if b.axis == tableRows {
					row := t.tab.rows[b.row]
					if dir > 0 {
						atEdge = e.caret == row[len(row)-1].end
					} else {
						atEdge = e.caret == row[0].start
					}
				}
				if atEdge {
					resume, row, axis, index, anchor, fromText, textMode := b.resume, b.row, b.axis, b.index, b.selectionAnchor, b.fromText, b.textMode
					e.tableBridge = nil
					e.modal.mode = resume
					e.selectTableAxis(t, axis, anchor, index, false)
					e.tableSelection.activeRow = row
					e.tableSelection.right = dir > 0
					e.tableSelection.fromText = fromText
					e.tableSelection.textMode = textMode
					return true
				}
			}
			return false
		}
		return false
	}
	s := e.tableSelection
	if s == nil || s.axis == tableWhole {
		return false
	}
	t := e.tableSelectionLeaf()
	if t == nil {
		return false
	}
	row, col, axis, index := s.activeRow, -1, tableColumns, -1
	inward := s.right && dir < 0 || !s.right && dir > 0
	if s.axis == tableColumns {
		col = s.active
		if !inward {
			col += dir
		}
		index = col
	} else {
		row = s.active
		if !inward {
			row += dir
		}
		if row < 0 || row >= len(t.tab.rows) {
			return true
		}
		axis, index = tableRows, row
		if inward {
			if dir > 0 {
				col = 0
			} else {
				col = len(t.tab.rows[row]) - 1
			}
		} else if dir > 0 {
			col = 0
		} else {
			col = len(t.tab.rows[row]) - 1
		}
	}
	if row < 0 || row >= len(t.tab.rows) || col < 0 || col >= len(t.tab.rows[row]) {
		// An outward motion from a structural edge must not fall through to
		// ordinary source stepping, whose table cell order can wrap visually.
		return s.axis == tableRows || s.axis == tableColumns
	}
	cell := t.tab.rows[row][col]
	resume := e.modal.mode
	if e.Keymap() != "default" && resume == "BlockVisual" {
		e.modal.mode = "Visual"
	}
	e.tableSelectDrag = nil
	e.tableBridge = &tableContentBridge{
		lineStart: t.lineStart, axis: axis, row: row, col: col, dir: dir,
		resume: resume, index: index, selectionAnchor: s.anchor, fromText: s.fromText, textMode: s.textMode,
	}
	textAnchor := e.anchor
	contentStart, contentEnd := e.tableSelectionContentRange(t, s)
	e.tableSelection = nil
	if dir > 0 {
		e.anchor, e.caret = contentStart, e.step(cell.start, 1, false)
	} else {
		e.anchor, e.caret = contentEnd, e.step(cell.end, -1, false)
	}
	if s.fromText {
		e.anchor = textAnchor
	}
	e.hasGoal = false
	e.wantReveal = true
	e.blinkStart = time.Now()
	e.invalidate()
	return true
}

// moveHorizontal records explicit navigation transitions. Opposite motion
// retraces the exact preceding state, including bridge identity and text source,
// instead of reconstructing a selection from a whole-row content range.
func (e *Editor) moveHorizontal(dir int, extend bool) {
	before := e.tableNavigationState()
	if n := len(e.tableNavigation); n > 0 {
		step := e.tableNavigation[n-1]
		if sameTableNavigationState(before, step.after) && dir == -step.dir {
			e.tableNavigation = e.tableNavigation[:n-1]
			e.restoreTableNavigation(step.before)
			return
		}
		if !sameTableNavigationState(before, step.after) {
			e.tableNavigation = nil
		}
	}
	e.tableNavigating = true
	defer func() { e.tableNavigating = false }()
	if !e.tableHorizontalTransition(dir) {
		e.setCaret(e.step(e.caret, dir, false), extend)
	}
	after := e.tableNavigationState()
	if !sameTableNavigationState(before, after) && (before.structure != nil || before.bridge != nil || after.structure != nil || after.bridge != nil || len(e.tableNavigation) > 0) {
		e.tableNavigation = append(e.tableNavigation, tableNavigationStep{before: before, after: after, dir: dir})
	}
}

func (e *Editor) tableSelectionSourceRange(t *leaf, s *tableSelection) (int, int, bool) {
	if t == nil || s == nil || t.tab == nil {
		return 0, 0, false
	}
	if s.axis == tableWhole || s.axis == tableColumns && s.first == 0 && s.last == t.tab.cols()-1 {
		return t.lineStart, blockEnd(e.src, t), true
	}
	if s.axis == tableRows {
		lines := e.tableLines(t)
		if len(lines) < 2 {
			return 0, 0, false
		}
		lineForRow := func(row int) int {
			if row == 0 {
				return 0
			}
			return row + 1
		}
		firstLine, lastLine := lineForRow(s.first), lineForRow(s.last)
		if firstLine >= len(lines) || lastLine >= len(lines) {
			return 0, 0, false
		}
		a, z := lines[firstLine][0], lines[lastLine][1]
		return a, z, true
	}
	if s.axis == tableColumns {
		if len(t.tab.rows) == 0 || len(t.tab.rows[0]) == 0 {
			return 0, 0, false
		}
		firstRow := t.tab.rows[0]
		lastRow := t.tab.rows[len(t.tab.rows)-1]
		first := firstRow[min(s.first, len(firstRow)-1)]
		last := lastRow[min(s.last, len(lastRow)-1)]
		return first.start, last.end, true
	}
	return 0, 0, false
}

func (e *Editor) tableRowBoundary(t *leaf, row int, right bool) int {
	lines := e.tableLines(t)
	line := row
	if row > 0 {
		line = row + 1
	}
	if line < 0 || line >= len(lines) {
		return t.lineStart
	}
	if right {
		return lines[line][1]
	}
	return lines[line][0]
}

func (e *Editor) tableSelectionContainsCell(t *leaf, row, col int) bool {
	s := e.tableSelection
	if s == nil || t == nil || s.lineStart != t.lineStart {
		return false
	}
	switch s.axis {
	case tableWhole:
		return true
	case tableRows:
		return row >= s.first && row <= s.last
	case tableColumns:
		return col >= s.first && col <= s.last
	}
	return false
}

// tableSelectionContentRange returns the rendered content span of a structural
// selection. It lets a structural selection downgrade into one ordinary text
// selection when horizontal motion enters adjacent content.
func (e *Editor) tableSelectionContentRange(t *leaf, s *tableSelection) (int, int) {
	if t == nil || s == nil || t.tab == nil || len(t.tab.rows) == 0 {
		return 0, 0
	}
	if s.axis == tableRows {
		first := t.tab.rows[min(s.first, len(t.tab.rows)-1)]
		last := t.tab.rows[min(s.last, len(t.tab.rows)-1)]
		if len(first) > 0 && len(last) > 0 {
			return first[0].start, last[len(last)-1].end
		}
	}
	if s.axis == tableColumns {
		firstRow := t.tab.rows[0]
		lastRow := t.tab.rows[len(t.tab.rows)-1]
		if len(firstRow) > 0 && len(lastRow) > 0 {
			firstCol := min(s.first, len(firstRow)-1)
			lastCol := min(s.last, len(lastRow)-1)
			return firstRow[firstCol].start, lastRow[lastCol].end
		}
	}
	a, z, _ := e.tableSelectionSourceRange(t, s)
	return a, z
}

func (e *Editor) tableSelectionCaretRect() (ui.Rect, bool) {
	s := e.tableSelection
	t := e.tableSelectionLeaf()
	if s == nil || t == nil || t.geom == nil || len(t.geom.rowY) < 2 {
		return ui.Rect{}, false
	}
	row := 0
	x := t.x
	if s.axis == tableColumns {
		col := max(0, min(s.active, len(t.geom.colX)-2))
		if s.right {
			col++
		}
		x += t.geom.colX[col]
		row = max(0, min(s.activeRow, min(len(t.tab.rows), len(t.geom.rowY)-1)-1))
	} else {
		row = max(0, min(s.active, min(len(t.tab.rows), len(t.geom.rowY)-1)-1))
		if s.axis == tableWhole {
			row = 0
			if s.right {
				row = len(t.tab.rows) - 1
			}
		}
		if s.right {
			x += t.geom.w
		}
	}
	return ui.Rect{X: x, Y: t.y + t.geom.rowY[row], W: 1, H: t.geom.rowY[row+1] - t.geom.rowY[row]}, true
}

func (e *Editor) tableBoundaryAtOffset(off int) (*leaf, int, bool, bool) {
	if e.phantom != nil && e.leafIsPhantom(off) {
		return nil, 0, false, false
	}
	for _, t := range e.leaves {
		if t.kind != kTable || t.tab == nil || t.geom == nil {
			continue
		}
		lines := e.tableLines(t)
		for row := range t.tab.rows {
			li := row
			if row > 0 {
				li++
			}
			if li >= len(lines) || row+1 >= len(t.geom.rowY) {
				continue
			}
			if off == lines[li][0] {
				return t, row, false, true
			}
			if off == lines[li][1] {
				return t, row, true, true
			}
		}
	}
	return nil, 0, false, false
}

func (e *Editor) moveStructuralRow(dir int, extend bool) bool {
	s := e.tableSelection
	if s == nil || (s.axis != tableRows && s.axis != tableWhole) {
		return false
	}
	t := e.tableSelectionLeaf()
	if t == nil {
		return false
	}
	active := s.active + dir
	if s.axis == tableWhole || active < 0 || active >= len(t.tab.rows) {
		focus := e.selectionFocus()
		a, z := e.selection()
		e.clearTableSelection()
		e.caret = focus
		e.anchor = focus
		if extend {
			e.anchor = a
			if dir < 0 {
				e.anchor = z
			}
		}
		e.leaveTableBoundary(t, dir, extend)
		return true
	}
	anchor := active
	if extend {
		anchor = s.anchor
	}
	right, fromText, textMode := s.right, s.fromText, s.textMode
	e.selectTableAxis(t, tableRows, anchor, active, false)
	e.tableSelection.right = right
	if extend {
		e.tableSelection.fromText = fromText
		e.tableSelection.textMode = textMode
	}
	return true
}

func (e *Editor) structuralSelectionForRange() bool {
	// Recompute from the current endpoints on every extended move. Keeping a
	// previous structural state here made a shrinking selection stay structural.
	e.tableSelection = nil
	if e.anchor == e.caret {
		return false
	}
	for _, t := range e.leaves {
		if t.kind != kTable || t.tab == nil {
			continue
		}
		lines := e.tableLines(t)
		// A document selection can contain text and table structures together.
		// Do not replace that envelope with a single row at the active edge.
		if e.anchor < t.lineStart || e.anchor > blockEnd(e.src, t) || e.caret < t.lineStart || e.caret > blockEnd(e.src, t) {
			continue
		}
		rowAtOffset := func(off int) (int, bool) {
			for row := range t.tab.rows {
				li := row
				if row > 0 {
					li = row + 1
				}
				if li < len(lines) && off >= lines[li][0] && off <= lines[li][1] {
					return row, true
				}
			}
			return 0, false
		}
		arow, aok := rowAtOffset(e.anchor)
		crow, cok := rowAtOffset(e.caret)
		if aok && cok {
			rowBoundary := func(row, off int) bool {
				li := row
				if row > 0 {
					li = row + 1
				}
				return li < len(lines) && (off == lines[li][0] || off == lines[li][1])
			}
			if rowBoundary(arow, e.anchor) || rowBoundary(crow, e.caret) {
				first, last := min(arow, crow), max(arow, crow)
				li := crow
				if crow > 0 {
					li++
				}
				right := li < len(lines) && e.caret == lines[li][1]
				s := &tableSelection{lineStart: t.lineStart, axis: tableRows, first: first, last: last, anchor: arow, active: crow, activeRow: crow, right: right, fromText: true, textMode: e.modal.mode}
				e.tableSelection = s
				return true
			}
		}
		for r := range t.tab.rows {
			li := r
			if r > 0 {
				li = r + 1
			}
			if li >= len(lines) {
				continue
			}
			ls, le := lines[li][0], lines[li][1]
			anchorLine, caretLine := lineStartAt(e.src, e.anchor), lineStartAt(e.src, e.caret)
			touches := e.anchor == ls || e.anchor == le || e.caret == ls || e.caret == le
			if touches && (anchorLine == ls || caretLine == ls) {
				s := &tableSelection{lineStart: t.lineStart, axis: tableRows, first: r, last: r, anchor: r, active: r, activeRow: r, fromText: true, textMode: e.modal.mode}
				e.tableSelection = s
				return true
			}
		}
		_, ar, ac := cellAt(t, e.anchor)
		_, cr, cc := cellAt(t, e.caret)
		if e.modal.mode == "BlockVisual" && ar >= 0 && cr >= 0 && ar != cr {
			lastRow := len(t.tab.rows) - 1
			topCol, bottomCol := -1, -1
			if ar == 0 && e.anchor <= t.tab.rows[ar][ac].start && cr == lastRow && e.caret >= t.tab.rows[cr][cc].end {
				topCol, bottomCol = ac, cc
			} else if cr == 0 && e.caret <= t.tab.rows[cr][cc].start && ar == lastRow && e.anchor >= t.tab.rows[ar][ac].end {
				topCol, bottomCol = cc, ac
			}
			if topCol >= 0 {
				first, last := min(topCol, bottomCol), max(topCol, bottomCol)
				right := e.caret >= t.tab.rows[cr][cc].end
				e.tableSelection = &tableSelection{lineStart: t.lineStart, axis: tableColumns, first: first, last: last, anchor: topCol, active: bottomCol, activeRow: cr, right: right, fromText: true, textMode: e.modal.mode}
				return true
			}
		}
	}
	return false
}

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
	if s.axis == tableRows || s.axis == tableWhole {
		r.align = append([]parser.Align(nil), t.tab.align...)
		if s.first > 0 && s.axis != tableWhole {
			for _, cell := range t.tab.rows[0] {
				r.header = append(r.header, e.src[cell.start:cell.end])
			}
		}
		for _, rowIndex := range e.selectedTableRows(t, s) {
			row := t.tab.rows[rowIndex]
			values := make([]string, len(row))
			for i, cell := range row {
				values[i] = e.src[cell.start:cell.end]
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
					values = append(values, e.src[row[c].start:row[c].end])
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
				case parser.AlignLeft:
					sep = ":---"
				case parser.AlignCenter:
					sep = ":---:"
				case parser.AlignRight:
					sep = "---:"
				}
				b.WriteString(" " + sep + " |")
			}
		}
	}
	return b.String()
}

// applyTableSelection performs a row or column edit in one undo step. The
// parser remains the authority for whether the result is still a GFM table.
func (e *Editor) applyTableSelection(op string) bool {
	s := e.tableSelection
	t := e.tableSelectionLeaf()
	if s == nil || t == nil {
		return false
	}
	reg := e.tableRegisterForSelection(t, s)
	e.modal.registerTable = reg
	if reg != nil {
		e.modal.register = reg.markdown()
		e.modal.registerLine = true
	}
	if op == "y" {
		focus := e.selectionFocus()
		e.Copy()
		e.tableSelection = nil
		e.tableSelectDrag = nil
		e.tableBridge = nil
		e.tableNavigation = nil
		e.anchor, e.caret = focus, focus
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
		a, z := lines[first][0], lines[last][1]
		if z < len(e.src) {
			z++
		}
		replacement := ""
		if op == "c" {
			replacement = "\n"
		}
		e.begin(stepEdit)
		e.replace(a, z, replacement)
		caret := a
		if op == "c" {
			caret++
		}
		e.commit(caret, caret)
		if op == "c" {
			e.modal.mode = "Insert"
		}
		return true
	}
	if s.axis == tableColumns {
		var b strings.Builder
		for li, ln := range lines {
			if li > 0 {
				b.WriteByte('\n')
			}
			pipes := splitRow(e.src, ln)
			if len(pipes) < 2 {
				b.WriteString(e.src[ln[0]:ln[1]])
				continue
			}
			b.WriteString(e.src[ln[0] : pipes[0]+1])
			written := 0
			for c := 0; c < t.tab.cols(); c++ {
				selected := c >= s.first && c <= s.last
				if op == "d" && selected {
					continue
				}
				if written > 0 {
					b.WriteString(" |")
				}
				value := ""
				if li == 1 {
					value = " --- "
					if c < len(t.tab.align) {
						switch t.tab.align[c] {
						case parser.AlignLeft:
							value = " :-- "
						case parser.AlignCenter:
							value = " :-: "
						case parser.AlignRight:
							value = " --: "
						}
					}
				} else if li == 0 && c < len(t.tab.rows[0]) {
					cell := t.tab.rows[0][c]
					value = e.src[cell.start:cell.end]
				} else if li > 1 && li-1 < len(t.tab.rows) && c < len(t.tab.rows[li-1]) {
					cell := t.tab.rows[li-1][c]
					value = e.src[cell.start:cell.end]
				}
				if selected && li != 1 {
					value = ""
				}
				b.WriteString(" ")
				b.WriteString(value)
				written++
			}
			if written > 0 {
				b.WriteString(" |")
			}
			if pipes[len(pipes)-1]+1 < ln[1] {
				b.WriteString(e.src[pipes[len(pipes)-1]+1 : ln[1]])
			}
		}
		a, z := lines[0][0], lines[len(lines)-1][1]
		e.begin(stepEdit)
		e.replace(a, z, b.String())
		e.commit(a, a)
		if op == "c" {
			e.modal.mode = "Insert"
			e.modal.blockInsert = &tableBlockInsert{lineStart: s.lineStart, firstCol: s.first}
		}
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
	// Replace the selected block with one copy per row in its leftmost column.
	var edits [][2]int
	var values []string
	for ri, row := range t.tab.rows {
		for c := s.first; c <= s.last && c < len(row); c++ {
			cell := row[c]
			edits = append(edits, [2]int{cell.start, cell.end})
			v := ""
			if c == s.first {
				v = text
			}
			values = append(values, v)
		}
		_ = ri
	}
	if len(edits) == 0 {
		return false
	}
	e.begin(stepEdit)
	for i := len(edits) - 1; i >= 0; i-- {
		e.replace(edits[i][0], edits[i][1], values[i])
	}
	caret := edits[0][0] + len(text)
	e.commit(caret, caret)
	e.modal.mode = "Insert"
	e.modal.blockInsert = &tableBlockInsert{lineStart: s.lineStart, firstCol: s.first, text: text}
	return true
}

func (e *Editor) typeTableBlock(text string) bool {
	b := e.modal.blockInsert
	if b == nil {
		return false
	}
	var t *leaf
	for _, l := range e.leaves {
		if l.kind == kTable && l.lineStart == b.lineStart {
			t = l
			break
		}
	}
	if t == nil {
		e.modal.blockInsert = nil
		return false
	}
	targets := make([]int, 0, len(t.tab.rows))
	for _, row := range t.tab.rows {
		if b.firstCol < len(row) {
			targets = append(targets, row[b.firstCol].end)
		}
	}
	if len(targets) == 0 {
		e.modal.blockInsert = nil
		return false
	}
	e.begin(stepTyping)
	for i := len(targets) - 1; i >= 0; i-- {
		e.replace(targets[i], targets[i], text)
	}
	b.text += text
	e.commit(targets[0]+len(text), targets[0]+len(text))
	return true
}

func (e *Editor) backspaceTableBlock() bool {
	b := e.modal.blockInsert
	if b == nil || b.text == "" {
		return b != nil
	}
	_, n := utf8.DecodeLastRuneInString(b.text)
	remove := b.text[len(b.text)-n:]
	var t *leaf
	for _, l := range e.leaves {
		if l.kind == kTable && l.lineStart == b.lineStart {
			t = l
			break
		}
	}
	if t == nil {
		e.modal.blockInsert = nil
		return false
	}
	targets := make([]int, 0, len(t.tab.rows))
	for _, row := range t.tab.rows {
		if b.firstCol < len(row) {
			targets = append(targets, row[b.firstCol].end)
		}
	}
	e.begin(stepDelete)
	for i := len(targets) - 1; i >= 0; i-- {
		start := targets[i] - len(remove)
		if start >= 0 && e.src[start:targets[i]] == remove {
			e.replace(start, targets[i], "")
		}
	}
	b.text = b.text[:len(b.text)-n]
	e.commit(e.caret, e.caret)
	return true
}

func (e *Editor) pasteTableRegister(before bool) bool {
	r := e.modal.registerTable
	if r == nil || len(r.rows) == 0 {
		return false
	}
	u := e.unitAt(e.caret)
	t := e.tableOf(u)
	if edge, _ := e.tableAtEdge(e.caret); t == nil && edge != nil {
		t = edge
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
				at = blockEnd(e.src, t)
			}
		}
		if at > 0 {
			prefix := e.src[:at]
			value = strings.Repeat("\n", max(0, 2-(len(prefix)-len(strings.TrimRight(prefix, "\n"))))) + value
		}
		if at < len(e.src) {
			suffix := e.src[at:]
			value += strings.Repeat("\n", max(0, 2-(len(suffix)-len(strings.TrimLeft(suffix, "\n")))))
		}
		e.begin(stepEdit)
		e.replace(at, at, value)
		e.commit(at+len(value), at+len(value))
		return true
	}
	lines := e.tableLines(t)
	if len(lines) < 2 {
		return false
	}
	_, row, col := cellAt(t, e.caret)
	if row < 0 {
		row, col = 0, 0
	}
	if r.axis == tableRows {
		at := lines[len(lines)-1][1]
		appendAfter := !before && row > 0
		if before {
			if row > 0 {
				at = lines[row+1][0]
			}
		} else if row > 0 && row+2 < len(lines) {
			at = lines[row+2][0]
			appendAfter = false
		}
		if row == 0 {
			at = lines[1][1]
			if at < len(e.src) {
				at++
			}
			appendAfter = false
		}
		prefix := e.src[t.lineStart:t.start]
		var b strings.Builder
		for i, values := range r.rows {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(prefix)
			b.WriteByte('|')
			for _, value := range values {
				b.WriteByte(' ')
				b.WriteString(value)
				b.WriteString(" |")
			}
		}
		value := b.String() + "\n"
		if appendAfter {
			value = "\n" + b.String()
		}
		e.begin(stepEdit)
		e.replace(at, at, value)
		e.commit(at+len(value), at+len(value))
		return true
	}
	if r.axis == tableColumns {
		col = max(0, min(col, t.tab.cols()-1))
		if !before {
			col++
		}
		var b strings.Builder
		for li, ln := range lines {
			if li > 0 {
				b.WriteByte('\n')
			}
			pipes := splitRow(e.src, ln)
			if len(pipes) < 2 {
				b.WriteString(e.src[ln[0]:ln[1]])
				continue
			}
			b.WriteString(e.src[ln[0] : pipes[0]+1])
			written := 0
			for c := 0; c <= t.tab.cols(); c++ {
				if c == col {
					for k := range r.rows[0] {
						if written > 0 {
							b.WriteString(" |")
						}
						value := ""
						if li == 1 {
							value = " --- "
							if k < len(r.align) {
								switch r.align[k] {
								case parser.AlignLeft:
									value = " :-- "
								case parser.AlignCenter:
									value = " :-: "
								case parser.AlignRight:
									value = " --: "
								}
							}
						} else if li == 0 && k < len(r.rows[0]) {
							value = r.rows[0][k]
						} else if li > 1 && li-1 < len(r.rows) && k < len(r.rows[li-1]) {
							value = r.rows[li-1][k]
						}
						b.WriteByte(' ')
						b.WriteString(value)
						written++
					}
				}
				if c == t.tab.cols() {
					break
				}
				if written > 0 {
					b.WriteString(" |")
				}
				value := ""
				if li == 1 {
					value = " --- "
				} else if li == 0 && c < len(t.tab.rows[0]) {
					cell := t.tab.rows[0][c]
					value = e.src[cell.start:cell.end]
				} else if li > 1 && li-1 < len(t.tab.rows) && c < len(t.tab.rows[li-1]) {
					cell := t.tab.rows[li-1][c]
					value = e.src[cell.start:cell.end]
				}
				b.WriteByte(' ')
				b.WriteString(value)
				written++
			}
			if written > 0 {
				b.WriteString(" |")
			}
		}
		for ri := len(t.tab.rows); ri < len(r.rows); ri++ {
			b.WriteByte('\n')
			b.WriteString(e.src[t.lineStart:t.start])
			b.WriteByte('|')
			written := 0
			for c := 0; c <= t.tab.cols(); c++ {
				if c == col {
					for k := range r.rows[ri] {
						if written > 0 {
							b.WriteString(" |")
						}
						b.WriteByte(' ')
						b.WriteString(r.rows[ri][k])
						written++
					}
				}
				if c == t.tab.cols() {
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
		a, z := lines[0][0], lines[len(lines)-1][1]
		e.begin(stepEdit)
		e.replace(a, z, b.String())
		e.commit(a, a)
		return true
	}
	return false
}
