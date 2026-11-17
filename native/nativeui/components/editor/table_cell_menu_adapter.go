package editor

// focusTableMenuCell gives menu actions the same table-cell focus semantics as
// Vim and pointer navigation. An existing region containing the menu cell is
// retained; choosing a menu outside it moves the active region to that cell.
func (e *Editor) focusTableMenuCell(t *leaf, row, col int) {
	// An ordinary text selection belongs to the document text layer. Do not
	// replace it with a structural cell selection just because a table menu ran.
	if e.Session.Selection().Anchor != e.Caret() && e.tableSelection == nil && e.tableBridge == nil {
		return
	}
	if e.tableSelection != nil && e.tableSelection.lineStart == t.lineStart && e.tableSelectionContainsCell(t, row, col) {
		return
	}
	e.selectTableCellRegion(t, row, col, row, col)
}
