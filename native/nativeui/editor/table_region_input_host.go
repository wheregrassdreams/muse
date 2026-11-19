package editor

import "muse/editor/input/vim"

func (e *Editor) tableRegionAtCaret() (*leaf, int, int, bool) {
	return e.tableCellAtSourceOffset(e.Caret())
}

func (e *Editor) tableCellAtSourceOffset(off int) (*leaf, int, int, bool) {
	for _, t := range e.leaves {
		if t.kind != kTable || t.tab == nil || off < t.lineStart || off > blockEnd(e.Source(), t) {
			continue
		}
		for row, cells := range t.tab.rows {
			for col, cell := range cells {
				if off >= cell.start && off <= cell.end {
					return t, row, col, true
				}
			}
		}
		row, col, ok := e.tablePositionAtSource(t, off)
		return t, row, col, ok
	}
	return nil, 0, 0, false
}

func (e *Editor) promoteVisualTextToCellRegion() bool {
	if !e.vimController().View().VimTextVisualActive ||
		e.tableSelection != nil && e.tableSelection.cellRegion ||
		e.tableBridge != nil || e.vimController().Mode() != vim.ModeVisual ||
		e.vimController().View().Linewise || e.Keymap() != "vim" {
		return false
	}
	selection := e.Session.Selection()
	anchor, focus := selection.Anchor, e.Caret()
	anchorTable, anchorRow, anchorCol, anchorOK := e.tableCellAtSourceOffset(anchor)
	focusTable, focusRow, focusCol, focusOK := e.tableCellAtSourceOffset(focus)
	if !anchorOK || !focusOK || anchorTable.lineStart != focusTable.lineStart || anchorRow == focusRow && anchorCol == focusCol {
		return false
	}
	if !e.selectTableCellRegion(anchorTable, anchorRow, anchorCol, focusRow, focusCol) {
		return false
	}
	// SelectCell records SelectionOriginVisualText from the active Vim mode;
	// the typed table selection carries that provenance after the bridge ends.
	e.vimController().SetTextVisualActive(false)
	return true
}
