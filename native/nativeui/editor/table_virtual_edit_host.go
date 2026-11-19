package editor

// tableVirtualEdge identifies the source side represented by a virtual cell
// cursor. The cursor is structural state and has no source byte offset.
type tableVirtualEdge uint8

const (
	tableVirtualLeading tableVirtualEdge = iota
	tableVirtualTrailing
)

// tableVirtualEditState projects a caret into the parsed table without
// changing the canonical Markdown source.
type tableVirtualEditState struct {
	lineStart  int
	row        int
	col        int
	edge       tableVirtualEdge
	axis       tableAxis
	postDelete bool
}

func (e *Editor) enterTableVirtualEdit(t *leaf, row, col int, edge tableVirtualEdge, axis tableAxis, postDelete bool) bool {
	if t == nil || t.kind != kTable || t.tab == nil || t.lineStart < 0 || row < 0 || col < 0 || row >= len(t.tab.rows) || col >= len(t.tab.rows[row]) {
		e.clearTableVirtualEdit()
		return false
	}
	e.tableVirtualEdit = &tableVirtualEditState{lineStart: t.lineStart, row: row, col: col, edge: edge, axis: axis, postDelete: postDelete}
	return true
}

func (e *Editor) updateTableVirtualEdit(row, col int, edge tableVirtualEdge, axis tableAxis, postDelete bool) bool {
	t, _, ok := e.resolveTableVirtualEdit()
	if !ok || row < 0 || col < 0 || row >= len(t.tab.rows) || col >= len(t.tab.rows[row]) {
		e.clearTableVirtualEdit()
		return false
	}
	e.tableVirtualEdit.row = row
	e.tableVirtualEdit.col = col
	e.tableVirtualEdit.edge = edge
	e.tableVirtualEdit.axis = axis
	e.tableVirtualEdit.postDelete = postDelete
	return true
}

func (e *Editor) clearTableVirtualEdit() { e.tableVirtualEdit = nil }

func (e *Editor) resolveTableVirtualEdit() (*leaf, tableVirtualEditState, bool) {
	e.sync()
	s := e.tableVirtualEdit
	if s == nil || s.lineStart < 0 {
		return nil, tableVirtualEditState{}, false
	}
	for _, l := range e.leaves {
		if l.kind != kTable || l.lineStart != s.lineStart || l.tab == nil || len(l.tab.rows) == 0 {
			continue
		}
		row, col := s.row, s.col
		if s.postDelete {
			// Structural deletion can remove the row that held a virtual caret.
			// Keep the semantic insertion target attached to the remaining edge
			// row instead of dropping to source-level block paste.
			row = max(0, min(row, len(l.tab.rows)-1))
			col = max(0, min(col, len(l.tab.rows[row])-1))
		} else if row < 0 || row >= len(l.tab.rows) || col < 0 || col >= len(l.tab.rows[row]) {
			return nil, tableVirtualEditState{}, false
		}
		resolved := *s
		resolved.row, resolved.col = row, col
		return l, resolved, true
	}
	return nil, tableVirtualEditState{}, false
}
