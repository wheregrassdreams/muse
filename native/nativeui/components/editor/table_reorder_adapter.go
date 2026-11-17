package editor

import (
	"math"

	tablemodel "muse/editor/blocks/table"
	tableui "muse/nativeui/components/editor/blocks/table"
)

// Handle dragging owns reordering only. Text dragging and keyboard structure
// extension keep their existing selection paths and never create this state.
type tableHandleDrag struct {
	lineStart, first, last, index, gap int
	axis                               tableAxis
	startX, startY, position           float32
	moving                             bool
	revision                           uint64
}

func (e *Editor) beginTableHandleDrag(t *leaf, axis tableAxis, index int, shift bool, x, y float32) {
	if t == nil || t.tab == nil || t.geom == nil {
		return
	}
	s := e.tableSelection
	if shift && s != nil && s.lineStart == t.lineStart && s.axis == axis {
		e.selectTableAxis(t, axis, s.anchor, index, false)
	} else if s == nil || s.lineStart != t.lineStart || s.axis != axis || index < s.first || index > s.last {
		e.selectTableAxis(t, axis, index, index, false)
	}
	s = e.tableSelection
	if s == nil {
		return
	}
	e.setSessionSelectionMode(false, false)
	e.vimController().SetLinewise(false)
	position := y
	if axis == tableColumns {
		position = x
	}
	e.tableHandleDrag = &tableHandleDrag{lineStart: t.lineStart, first: s.first, last: s.last, index: index, axis: axis, gap: index, startX: x, startY: y, position: position, revision: e.Session.Revision()}
	e.wantReveal = false
	e.invalidate()
}

func (e *Editor) updateTableHandleDrag(x, y float32) {
	d := e.tableHandleDrag
	if d == nil {
		return
	}
	t := e.tableSelectionLeaf()
	if t == nil || t.geom == nil || t.lineStart != d.lineStart || e.Session.Revision() != d.revision {
		e.tableHandleDrag = nil
		return
	}
	if math.Abs(float64(x-d.startX))+math.Abs(float64(y-d.startY)) >= 4 {
		d.moving = true
	}
	edges, p := t.geom.rowY, y-t.y
	if d.axis == tableColumns {
		edges, p = t.geom.colX, x-tableContentX(t)
		d.position = max(t.x, min(x, t.x+tableViewportWidth(t)))
	} else {
		d.position = max(t.y, min(y, t.y+t.h))
	}
	d.gap = len(edges) - 1
	for i := 0; i+1 < len(edges); i++ {
		if p < (edges[i]+edges[i+1])/2 {
			d.gap = i
			break
		}
	}
	e.invalidate()
}

func (e *Editor) finishTableHandleDrag(canceled bool) {
	d := e.tableHandleDrag
	e.tableHandleDrag = nil
	if d != nil && !canceled && d.moving && d.revision == e.Session.Revision() {
		e.reorderTable(d.lineStart, d.axis, d.first, d.last, d.gap)
	}
	e.invalidate()
}

func (e *Editor) reorderTable(lineStart int, axis tableAxis, first, last, gap int) bool {
	e.layoutIfNeeded()
	t := e.tableByLineStart(lineStart)
	if t == nil || axis != tableRows && axis != tableColumns {
		return false
	}
	horizontal := t.horizontal.Offset
	anchor, active, activeRow, right := first, last, 0, true
	if s := e.tableSelection; s != nil && s.lineStart == lineStart && s.axis == axis {
		anchor, active, activeRow, right = s.anchor, s.active, s.activeRow, s.right
	}
	modelAxis := tablemodel.Rows
	if axis == tableColumns {
		modelAxis = tablemodel.Columns
	}
	var at int
	didChange := false
	e.actCaretStill(func() {
		kind := tableui.ReorderRows
		if modelAxis == tablemodel.Columns {
			kind = tableui.ReorderColumns
		}
		result, err := e.tableUI().Dispatch(tableui.Action{Kind: kind, LineStart: lineStart, Axis: modelAxis, First: first, Last: last, Gap: gap})
		if err != nil || !result.Changed {
			return
		}
		didChange = true
		at = result.Insertion
		e.layoutIfNeeded()
		updated := e.tableByLineStart(lineStart)
		if updated == nil {
			return
		}
		e.selectTableAxis(updated, axis, at, at+last-first, false)
		s := e.tableSelection
		s.anchor = at + max(0, min(anchor-first, last-first))
		s.active = at + max(0, min(active-first, last-first))
		s.right = right
		s.activeRow = activeRow
		if axis == tableRows {
			s.activeRow = s.active
		}
		e.Session.UpdateLastHistoryAfter()
	})
	if !didChange {
		return false
	}
	e.preserveTableHorizontal(lineStart, horizontal)
	return true
}

// Source edits rebuild table leaves. Keep the user's displayed horizontal
// viewport across a control action, clamped if the new table is narrower.
func (e *Editor) preserveTableHorizontal(lineStart int, offset float32) {
	e.layoutIfNeeded()
	for _, l := range e.leaves {
		if l.kind == kTable && l.lineStart == lineStart {
			e.setBlockHorizontal(l, offset)
			e.layoutIfNeeded()
			return
		}
	}
}
