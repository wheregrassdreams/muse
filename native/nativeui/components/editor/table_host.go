package editor

import (
	tablemodel "muse/editor/blocks/table"
	"muse/editor/internal/core"
)

// editorTableHost adapts the editor's private parser/history state to the
// table subsystem's small source-view and transaction contract.
type editorTableHost struct{ editor *Editor }

func (h editorTableHost) TableView(lineStart int) (tablemodel.DocumentView, bool) {
	e := h.editor
	e.sync()
	t := e.tableByLineStart(lineStart)
	if t == nil {
		return tablemodel.DocumentView{}, false
	}
	return documentViewForTable(e, t), true
}

func (h editorTableHost) TextSelection() core.Selection {
	return h.editor.TransactionSelection()
}

func (h editorTableHost) DocumentSource() string { return h.editor.Source() }

func (h editorTableHost) Apply(transaction core.Transaction) error {
	return h.editor.ApplyTransaction(transaction)
}

func documentViewForTable(e *Editor, t *leaf) tablemodel.DocumentView {
	view := tablemodel.DocumentView{
		LineStart:  t.lineStart,
		End:        blockEnd(e.src, t),
		Source:     e.src,
		Prefix:     e.src[t.lineStart:t.start],
		Rows:       make([][]core.TextRange, len(t.tab.rows)),
		Lines:      make([]tablemodel.LineView, 0),
		Alignments: make([]tablemodel.Alignment, len(t.tab.align)),
	}
	view.ContinuationPrefix = blankMarkers(view.Prefix)
	for col, alignment := range t.tab.align {
		view.Alignments[col] = tablemodel.Alignment(alignment)
	}
	for _, line := range e.tableLines(t) {
		view.Lines = append(view.Lines, tablemodel.LineView{
			Range: core.TextRange{Start: line[0], End: line[1]},
			Pipes: splitRow(e.src, line),
		})
	}
	for row, cells := range t.tab.rows {
		view.Rows[row] = make([]core.TextRange, len(cells))
		for col, cell := range cells {
			view.Rows[row][col] = core.TextRange{Start: cell.start, End: cell.end}
		}
	}
	return view
}
