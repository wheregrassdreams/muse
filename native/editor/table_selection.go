package editor

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	tablemodel "muse/editor/blocks/table"
	"muse/editor/internal/core"
	syntax "muse/markdown/syntax"
)

type tableAxis uint8

const (
	tableRows tableAxis = iota + 1
	tableColumns
	tableWhole
	tableCells
)

// tableSelection is a semantic selection. first/last are inclusive row or
// column indexes; a table selection uses the whole table.
type tableSelection struct {
	lineStart int
	// Cell coordinates are the canonical active table region. axis/first/last
	// remain compatibility fields for structural operations and legacy callers.
	anchorRow, anchorCol int
	focusRow, focusCol   int
	cellRegion           bool
	axis                 tableAxis
	first                int
	last                 int
	anchor               int
	active               int
	activeRow            int
	right                bool
	fromText             bool
	textMode             string
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

// tableNavigationState is the editor's immutable table state value. Text
// endpoints stay separate from table structure; a bridge owns its text focus.
func (e *Editor) tableNavigationState() tablemodel.Snapshot {
	return tablemodel.Snapshot{
		Mode: e.modal.mode,
		TextSelection: core.Selection{
			Anchor: e.anchor, Head: e.caret,
			Inclusive: e.selectionInclusive, Linewise: e.selectionLinewise,
		},
		Selection:      tableSelectionSnapshot(e.tableSelection),
		DocumentRegion: tableSelectionSnapshot(e.tableDocumentRegion),
		Bridge:         tableBridgeSnapshot(e.tableBridge),
	}
}

func (e *Editor) tableSnapshot() tablemodel.Snapshot { return e.tableNavigationState() }

func tableSelectionSnapshot(s *tableSelection) *tablemodel.Selection {
	if s == nil {
		return nil
	}
	return &tablemodel.Selection{
		LineStart:  s.lineStart,
		Anchor:     tablemodel.Cell{Row: s.anchorRow, Col: s.anchorCol},
		Focus:      tablemodel.Cell{Row: s.focusRow, Col: s.focusCol},
		CellRegion: s.cellRegion,
		Axis:       tablemodel.Axis(s.axis), First: s.first, Last: s.last,
		LegacyAnchor: s.anchor,
		Active:       s.active, ActiveRow: s.activeRow, Right: s.right,
		FromText: s.fromText, TextMode: s.textMode,
	}
}

func tableSelectionFromSnapshot(s *tablemodel.Selection) *tableSelection {
	if s == nil {
		return nil
	}
	return &tableSelection{
		lineStart: s.LineStart,
		anchorRow: s.Anchor.Row, anchorCol: s.Anchor.Col,
		focusRow: s.Focus.Row, focusCol: s.Focus.Col,
		cellRegion: s.CellRegion,
		axis:       tableAxis(s.Axis), first: s.First, last: s.Last,
		anchor: s.LegacyAnchor,
		active: s.Active, activeRow: s.ActiveRow, right: s.Right,
		fromText: s.FromText, textMode: s.TextMode,
	}
}

func tableBridgeSnapshot(s *tableContentBridge) *tablemodel.ContentBridge {
	if s == nil {
		return nil
	}
	return &tablemodel.ContentBridge{
		LineStart: s.lineStart, Axis: tablemodel.Axis(s.axis), Row: s.row, Col: s.col,
		Direction: s.dir, Resume: s.resume, Index: s.index,
		SelectionAnchor: s.selectionAnchor, FromText: s.fromText, TextMode: s.textMode,
	}
}

func tableBridgeFromSnapshot(s *tablemodel.ContentBridge) *tableContentBridge {
	if s == nil {
		return nil
	}
	return &tableContentBridge{
		lineStart: s.LineStart, axis: tableAxis(s.Axis), row: s.Row, col: s.Col,
		dir: s.Direction, resume: s.Resume, index: s.Index,
		selectionAnchor: s.SelectionAnchor, fromText: s.FromText, textMode: s.TextMode,
	}
}

func (e *Editor) restoreTableSnapshot(s tablemodel.Snapshot) {
	e.restoreTableNavigation(s)
}

func sameTableNavigationState(a, b tablemodel.Snapshot) bool { return tablemodel.Equal(a, b) }

func (e *Editor) restoreTableNavigation(s tablemodel.Snapshot) {
	e.modal.mode = s.Mode
	e.anchor, e.caret = s.TextSelection.Anchor, s.TextSelection.Head
	e.selectionInclusive = s.TextSelection.Inclusive
	e.selectionLinewise = s.TextSelection.Linewise
	e.tableSelection, e.tableBridge = nil, nil
	e.tableDocumentRegion = nil
	if s.Selection != nil {
		e.tableSelection = tableSelectionFromSnapshot(s.Selection)
	}
	if s.Bridge != nil {
		e.tableBridge = tableBridgeFromSnapshot(s.Bridge)
	}
	if s.DocumentRegion != nil {
		e.tableDocumentRegion = tableSelectionFromSnapshot(s.DocumentRegion)
	}
	e.hasGoal = false
	e.wantReveal = true
	e.blinkStart = time.Now()
	e.invalidate()
}

// replayTableNavigation restores the complete prior selection state when the
// latest cell transition is followed by its inverse direction.
func (e *Editor) replayTableNavigation(rowDelta, colDelta int) bool {
	if restored, ok := e.tableController.Replay(e.tableNavigationState(), rowDelta, colDelta); ok {
		e.restoreTableNavigation(restored)
		return true
	}
	return false
}

func (e *Editor) recordTableNavigation(before tablemodel.Snapshot, rowDelta, colDelta int) {
	e.tableController.Record(before, e.tableNavigationState(), rowDelta, colDelta)
}

// selectionFocus is the active variant's focus, never the dormant text head.
func (e *Editor) selectionFocus() int {
	s, t := e.tableSelection, e.tableSelectionLeaf()
	if s != nil && t != nil {
		if s.axis == tableCells {
			row := max(0, min(s.focusRow, len(t.tab.rows)-1))
			col := max(0, min(s.focusCol, len(t.tab.rows[row])-1))
			cell := t.tab.rows[row][col]
			return cell.start
		}
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
	axis         tableAxis
	rows         [][]string
	align        []syntax.IncrementalAlign
	header       []string // source header when the selected rows exclude it
	whole        bool
	region       bool // rows are an arbitrary rectangular cell matrix
	sourceHeader bool
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

// tablePositionAtSource maps parser-owned table line boundaries to structural
// coordinates. The source offset identifies a parsed line only; row and cell
// shape still come from the table projection, never from pipe or cell text.
func (e *Editor) tablePositionAtSource(t *leaf, off int) (row, col int, ok bool) {
	if t == nil || t.tab == nil || len(t.tab.rows) == 0 {
		return 0, 0, false
	}
	lines := e.tableLines(t)
	for line, span := range lines {
		if off < span[0] || off > span[1] {
			continue
		}
		row = 0
		switch {
		case line == 0 || line == 1:
			// The separator belongs to the header boundary.
			row = 0
		default:
			row = line - 1
		}
		row = max(0, min(row, len(t.tab.rows)-1))
		return row, 0, len(t.tab.rows[row]) > 0
	}
	if off == t.lineStart {
		return 0, 0, len(t.tab.rows[0]) > 0
	}
	if off == blockEnd(e.src, t) {
		row = len(t.tab.rows) - 1
		return row, max(0, len(t.tab.rows[row])-1), len(t.tab.rows[row]) > 0
	}
	return 0, 0, false
}

func (e *Editor) selectTableAxis(t *leaf, axis tableAxis, first, last int, extend bool) {
	if e.block != nil {
		e.block = nil
	}
	e.finishBlockInput()
	if !e.tableNavigating {
		e.tableController.ResetNavigation()
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
	s := &tableSelection{lineStart: t.lineStart, axis: axis, first: first, last: last, anchor: anchor, active: active, activeRow: 0, right: true, textMode: e.modal.mode}
	s.anchorRow, s.focusRow, s.anchorCol, s.focusCol = 0, 0, 0, 0
	switch axis {
	case tableRows:
		s.anchorRow, s.focusRow = anchor, active
		s.anchorCol, s.focusCol = 0, max(0, t.tab.cols()-1)
	case tableColumns:
		s.anchorRow, s.focusRow = 0, max(0, len(t.tab.rows)-1)
		s.anchorCol, s.focusCol = anchor, active
	case tableWhole:
		s.focusRow, s.focusCol = max(0, len(t.tab.rows)-1), max(0, t.tab.cols()-1)
	}
	e.tableSelection = s
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

// selectTableCellRegion updates the one active table selection using parser
// cell coordinates. It intentionally leaves ordinary text endpoints dormant.
func (e *Editor) selectTableCellRegion(t *leaf, anchorRow, anchorCol, focusRow, focusCol int) bool {
	if t == nil || t.tab == nil || len(t.tab.rows) == 0 || t.tab.cols() == 0 {
		return false
	}
	anchorRow = max(0, min(anchorRow, len(t.tab.rows)-1))
	focusRow = max(0, min(focusRow, len(t.tab.rows)-1))
	anchorCol = max(0, min(anchorCol, t.tab.cols()-1))
	focusCol = max(0, min(focusCol, t.tab.cols()-1))
	e.clearTableSelectionSession()
	// The region owns table presentation while the document endpoints retain
	// any selected text outside the table. Clear incompatible linewise/text flags.
	e.selectionInclusive = false
	e.selectionLinewise = false
	e.modal.sequence.Linewise = false
	top, bottom := min(anchorRow, focusRow), max(anchorRow, focusRow)
	left, right := min(anchorCol, focusCol), max(anchorCol, focusCol)
	axis, first, last := tableCells, top, bottom
	switch {
	case top == 0 && left == 0 && bottom == len(t.tab.rows)-1 && right == t.tab.cols()-1:
		axis, first, last = tableWhole, 0, len(t.tab.rows)-1
	case left == 0 && right == t.tab.cols()-1:
		axis, first, last = tableRows, top, bottom
	case top == 0 && bottom == len(t.tab.rows)-1:
		axis, first, last = tableColumns, left, right
	}
	compatAnchor, compatActive := anchorCol, focusCol
	if axis == tableRows || axis == tableWhole {
		compatAnchor, compatActive = anchorRow, focusRow
	}
	e.tableSelection = &tableSelection{lineStart: t.lineStart, axis: axis, anchorRow: anchorRow, anchorCol: anchorCol, focusRow: focusRow, focusCol: focusCol, cellRegion: true, first: first, last: last, anchor: compatAnchor, active: compatActive, activeRow: focusRow, right: true, textMode: e.modal.mode}
	e.tableBridge = nil
	if !e.tableNavigating {
		e.tableController.ResetNavigation()
	}
	e.wantReveal = true
	e.invalidate()
	return true
}

func (e *Editor) tableSelectionCellRegion(s *tableSelection) (tableCellRegion, bool) {
	if s == nil || !s.cellRegion {
		return tableCellRegion{}, false
	}
	return tableCellRegion{lineStart: s.lineStart, anchorRow: s.anchorRow, anchorCol: s.anchorCol, focusRow: s.focusRow, focusCol: s.focusCol}, true
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
	e.tableHandleDrag = nil
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
	if e.replayTableNavigation(0, dir) {
		return
	}
	e.tableNavigating = true
	defer func() { e.tableNavigating = false }()
	if !e.tableHorizontalTransition(dir) {
		e.setCaret(e.step(e.caret, dir, false), extend)
	}
	e.recordTableNavigation(before, 0, dir)
}

func (e *Editor) tableSelectionSourceRange(t *leaf, s *tableSelection) (int, int, bool) {
	if t == nil || s == nil || t.tab == nil {
		return 0, 0, false
	}
	if s.axis == tableWhole || s.axis == tableColumns && s.first == 0 && s.last == t.tab.cols()-1 {
		return t.lineStart, blockEnd(e.src, t), true
	}
	if s.cellRegion && s.axis == tableCells {
		r, ok := e.tableSelectionCellRegion(s)
		if !ok {
			return 0, 0, false
		}
		top, left, bottom, right := r.bounds()
		if top >= len(t.tab.rows) || bottom >= len(t.tab.rows) || left >= len(t.tab.rows[top]) || right >= len(t.tab.rows[bottom]) {
			return 0, 0, false
		}
		return t.tab.rows[top][left].start, t.tab.rows[bottom][right].end, true
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
	return tableSelectionContainsCell(e.tableSelection, t, row, col)
}

func tableSelectionContainsCell(s *tableSelection, t *leaf, row, col int) bool {
	if s == nil || t == nil || s.lineStart != t.lineStart {
		return false
	}
	switch s.axis {
	case tableCells:
		r := tableCellRegion{lineStart: s.lineStart, anchorRow: s.anchorRow, anchorCol: s.anchorCol, focusRow: s.focusRow, focusCol: s.focusCol}
		top, left, bottom, right := r.bounds()
		return row >= top && row <= bottom && col >= left && col <= right
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
	x := tableContentX(t)
	if s.cellRegion {
		row = max(0, min(s.focusRow, min(len(t.tab.rows), len(t.geom.rowY)-1)-1))
		col := max(0, min(s.focusCol, len(t.geom.colX)-2))
		x += t.geom.colX[col]
	} else if s.axis == tableColumns {
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
	suspended := cloneTableSelection(e.tableDocumentRegion)
	e.tableSelection = nil
	if e.anchor == e.caret {
		return false
	}
	for _, t := range e.leaves {
		if t.kind != kTable || t.tab == nil {
			continue
		}
		// A region suspended while the Visual caret was outside its table is
		// part of the same selection session. Re-entering the table resumes it
		// from the saved anchor and the parser-owned cell containing the caret;
		// the document endpoints continue to own any text outside the table.
		if suspended != nil && suspended.fromText && suspended.lineStart == t.lineStart && suspended.textMode == e.modal.mode {
			focusTable, focusRow, focusCol, focusOK := e.tableCellAtSourceOffset(e.caret)
			if focusOK && focusTable.lineStart == t.lineStart {
				if e.selectTableCellRegion(t, suspended.anchorRow, suspended.anchorCol, focusRow, focusCol) {
					e.tableSelection.fromText = suspended.fromText
					e.tableSelection.textMode = suspended.textMode
					e.tableDocumentRegion = nil
					return true
				}
			}
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
	if s.cellRegion {
		region, ok := e.tableSelectionCellRegion(s)
		if !ok {
			return nil
		}
		matrix := e.tableCellMatrix(t, region)
		if matrix == nil {
			return nil
		}
		top, _, _, _ := region.bounds()
		return &tableRegister{axis: s.axis, whole: r.whole, rows: matrix.Cells, align: parserAlignmentsFromTable(matrix.Alignments), region: true, sourceHeader: top == 0}
	}
	if s.axis == tableRows || s.axis == tableWhole {
		r.align = append([]syntax.IncrementalAlign(nil), t.tab.align...)
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
		top, left, bottom, right := region.bounds()
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
				e.modal.setTableRegister(e.tableRegisterForSelection(t, s))
				if !e.clearTableCellRegion(t, region) {
					return false
				}
				if op == "c" {
					e.modal.mode = "Insert"
				} else {
					e.modal.mode = "Normal"
				}
				e.tableSelection = nil
				e.tableBridge = nil
				return true
			}
		}
	}
	reg := e.tableRegisterForSelection(t, s)
	e.modal.setTableRegister(reg)
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
		e.tableController.ResetNavigation()
		e.anchor, e.caret = focus, focus
		e.modal.sequence.Linewise = false
		e.selectionInclusive = false
		e.selectionLinewise = false
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
		changed, _, err := e.tableController.DeleteRows(editorTableHost{editor: e}, s.lineStart, s.first, s.last, s.axis == tableWhole, op == "c")
		if err != nil || !changed {
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
			e.modal.mode = "Insert"
		}
		e.modal.sequence.Linewise = false
		e.selectionInclusive = false
		e.selectionLinewise = false
		return true
	}
	if s.axis == tableColumns {
		virtualLineStart := t.lineStart
		virtualRow := max(0, min(s.activeRow, len(t.tab.rows)-1))
		virtualCol := s.first
		keepVirtual := op == "d"
		changed, _, err := e.tableController.DeleteColumns(editorTableHost{editor: e}, s.lineStart, s.first, s.last, op == "c")
		if err != nil || !changed {
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
			e.modal.mode = "Insert"
			e.modal.blockInsert = &tableBlockInsert{lineStart: s.lineStart, firstCol: s.first}
		}
		e.modal.sequence.Linewise = false
		e.selectionInclusive = false
		e.selectionLinewise = false
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
	changed, _, err := e.tableController.TypeColumnSelection(editorTableHost{editor: e}, s.lineStart, s.first, s.last, text)
	if err != nil || !changed {
		return false
	}
	e.modal.mode = "Insert"
	e.modal.blockInsert = &tableBlockInsert{lineStart: s.lineStart, firstCol: s.first, text: text}
	return true
}

func (e *Editor) typeTableBlock(text string) bool {
	b := e.modal.blockInsert
	if b == nil {
		return false
	}
	changed, _, err := e.tableController.TypeBlock(editorTableHost{editor: e}, b.lineStart, b.firstCol, text)
	if err != nil || !changed {
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
	if changed, err := e.tableController.BackspaceBlock(editorTableHost{editor: e}, b.lineStart, b.firstCol, remove); err != nil || !changed {
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
		u := e.unitAt(e.caret)
		if u != nil {
			t = e.tableOf(u)
			if t != nil {
				_, row, col = cellAt(t, e.caret)
			}
		}
		if t == nil {
			if edge, after := e.tableAtEdge(e.caret); edge != nil {
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
				if candidate.kind != kTable || candidate.tab == nil || e.caret < candidate.lineStart || e.caret > blockEnd(e.src, candidate) {
					continue
				}
				t = candidate
				row, col, _ = e.tablePositionAtSource(t, e.caret)
				break
			}
		} else if row < 0 || col < 0 {
			row, col, _ = e.tablePositionAtSource(t, e.caret)
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
				at = blockEnd(e.src, t)
			}
		}
		changed, err := e.tableController.PasteBlock(editorTableHost{editor: e}, at, value)
		if err != nil || !changed {
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
		region := tableCellRegion{lineStart: t.lineStart, anchorRow: row, anchorCol: startCol, focusRow: row, focusCol: startCol}
		if e.replaceTableCellRegion(t, region, &tableCellMatrix{Cells: r.rows, Alignments: tableAlignmentsFromParser(r.align)}) {
			e.clearTableVirtualEdit()
			return true
		}
		return false
	}
	if r.axis == tableRows {
		changed, insertedRow, err := e.tableController.PasteRows(editorTableHost{editor: e}, t.lineStart, row, before, r.rows)
		if err != nil || !changed {
			return false
		}
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
		return true
	}
	if r.axis == tableColumns {
		virtualRow := row
		col = max(0, min(col, t.tab.cols()-1))
		if !before {
			col++
		}
		matrix := tablemodel.Matrix{Cells: r.rows, Alignments: tableAlignmentsFromParser(r.align)}
		changed, _, err := e.tableController.PasteColumns(editorTableHost{editor: e}, virtualLineStart, virtualRow, col, matrix)
		if err != nil || !changed {
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
		return true
	}
	return false
}
