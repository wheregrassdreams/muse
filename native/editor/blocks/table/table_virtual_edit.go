package mdedit

// tableVirtualEdge identifies which side of a virtual cell position owns the
// cursor. It is structural state; it is not a source byte offset.
type tableVirtualEdge uint8

const (
	tableVirtualLeading tableVirtualEdge = iota
	tableVirtualTrailing
)

// tableVirtualEditState is an ephemeral projection of the parsed Markdown
// table. The source remains canonical; this state only preserves a useful
// structural focus when a source offset cannot identify a cell (for example,
// immediately after deleting rows).
type tableVirtualEditState struct {
	lineStart  int
	row        int
	col        int
	edge       tableVirtualEdge
	axis       tableAxis
	postDelete bool
}

// enterTableVirtualEdit starts a structural editing session at a parsed table
// position. Invalid table positions clear any previous session.
func (e *Editor) enterTableVirtualEdit(t *leaf, row, col int, edge tableVirtualEdge, axis tableAxis, postDelete bool) bool {
	if t == nil || t.kind != kTable || t.tab == nil || t.lineStart < 0 || row < 0 || col < 0 || row >= len(t.tab.rows) || col >= len(t.tab.rows[row]) {
		e.clearTableVirtualEdit()
		return false
	}
	e.tableVirtualEdit = &tableVirtualEditState{
		lineStart:  t.lineStart,
		row:        row,
		col:        col,
		edge:       edge,
		axis:       axis,
		postDelete: postDelete,
	}
	return true
}

// updateTableVirtualEdit changes the structural cursor and operation context
// of an active session. It fails and clears the session if the table or
// requested position no longer exists.
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

func (e *Editor) clearTableVirtualEdit() {
	e.tableVirtualEdit = nil
}

// resolveTableVirtualEdit validates the stored source anchor against the
// current parse and returns the table and a bounded structural cursor. It
// never guesses a table from the text caret or silently retargets a different
// table when the anchor is stale.
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
