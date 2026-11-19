package editor

import (
	"time"

	tablemodel "github.com/wheregrassdreams/muse-editor/markdown/table"
	helix "github.com/wheregrassdreams/muse-editor/text/helix"
)

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
				atEdge := dir > 0 && e.Caret() == cell.end || dir < 0 && e.Caret() == cell.start
				if b.axis == tableRows {
					row := t.tab.rows[b.row]
					if dir > 0 {
						atEdge = e.Caret() == row[len(row)-1].end
					} else {
						atEdge = e.Caret() == row[0].start
					}
				}
				if atEdge {
					resume, row, axis, index, anchor, origin := b.resume, b.row, b.axis, b.index, b.selectionAnchor, b.origin
					e.tableBridge = nil
					e.modalSetMode(helix.Mode(resume))
					e.selectTableAxis(t, axis, anchor, index, false)
					e.tableSelection.activeRow = row
					e.tableSelection.right = dir > 0
					e.tableSelection.origin = origin
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
	resume := string(e.vimController().Mode())
	if e.Keymap() != "default" && resume == "BlockVisual" {
		e.modalSetMode(helix.ModeVisual)
	}
	e.tableHandleDrag = nil
	e.tableBridge = &tableContentBridge{
		lineStart: t.lineStart, axis: axis, row: row, col: col, dir: dir,
		resume: resume, index: index, selectionAnchor: s.anchor, origin: s.origin,
	}
	textAnchor := e.Session.Selection().Anchor
	contentStart, contentEnd := e.tableSelectionContentRange(t, s)
	e.tableSelection = nil
	if dir > 0 {
		e.setSessionSelection(contentStart, e.step(cell.start, 1, false))
	} else {
		e.setSessionSelection(contentEnd, e.step(cell.end, -1, false))
	}
	if s.origin != tablemodel.SelectionOriginTable {
		e.setSessionAnchor(textAnchor)
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
		e.setCaret(e.step(e.Caret(), dir, false), extend)
	}
	e.recordTableNavigation(before, 0, dir)
}
