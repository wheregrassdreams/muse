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
		if s.row < 0 || s.row >= len(l.tab.rows) || s.col < 0 || s.col >= len(l.tab.rows[s.row]) {
			return nil, tableVirtualEditState{}, false
		}
		return l, *s, true
	}
	return nil, tableVirtualEditState{}, false
}
