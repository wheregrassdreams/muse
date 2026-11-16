package editor

import (
	tablemodel "muse/editor/blocks/table"
	syntax "muse/markdown/syntax"
)

// tableCellRegion is the key-independent rectangular cell selection value.
// Coordinates are inclusive and always refer to parser-projected table cells.
type tableCellRegion struct {
	lineStart            int
	anchorRow, anchorCol int
	focusRow, focusCol   int
}

func (r tableCellRegion) bounds() (top, left, bottom, right int) {
	top, bottom = r.anchorRow, r.focusRow
	left, right = r.anchorCol, r.focusCol
	if top > bottom {
		top, bottom = bottom, top
	}
	if left > right {
		left, right = right, left
	}
	return
}

type tableCellMatrix = tablemodel.Matrix

func tableAlignmentsFromParser(values []syntax.IncrementalAlign) []tablemodel.Alignment {
	result := make([]tablemodel.Alignment, len(values))
	for i, value := range values {
		result[i] = tablemodel.Alignment(value)
	}
	return result
}

func parserAlignmentsFromTable(values []tablemodel.Alignment) []syntax.IncrementalAlign {
	result := make([]syntax.IncrementalAlign, len(values))
	for i, value := range values {
		result[i] = syntax.IncrementalAlign(value)
	}
	return result
}

func (e *Editor) tableCellMatrix(t *leaf, region tableCellRegion) *tableCellMatrix {
	if t == nil || t.tab == nil || t.lineStart != region.lineStart {
		return nil
	}
	view := documentViewForTable(e, t)
	selection := tablemodel.Selection{
		LineStart:  region.lineStart,
		Anchor:     tablemodel.Cell{Row: region.anchorRow, Col: region.anchorCol},
		Focus:      tablemodel.Cell{Row: region.focusRow, Col: region.focusCol},
		CellRegion: true,
	}
	matrix, ok := tablemodel.CopyRegion(view, selection)
	if !ok {
		return nil
	}
	_, left, _, right := region.bounds()
	matrix.Alignments = make([]tablemodel.Alignment, right-left+1)
	for c := left; c <= right; c++ {
		matrix.Alignments[c-left] = tablemodel.Alignment(alignAt(t.tab.align, c))
	}
	return &matrix
}

// clearTableCellRegion clears cell contents while preserving table dimensions.
// All source edits are committed as one normal editor transaction.
func (e *Editor) clearTableCellRegion(t *leaf, region tableCellRegion) bool {
	if t == nil || t.tab == nil || t.lineStart != region.lineStart {
		return false
	}
	selection := tablemodel.Selection{
		LineStart:  region.lineStart,
		Anchor:     tablemodel.Cell{Row: region.anchorRow, Col: region.anchorCol},
		Focus:      tablemodel.Cell{Row: region.focusRow, Col: region.focusCol},
		CellRegion: true,
	}
	changed, err := e.tableController.ClearRegion(editorTableHost{editor: e}, selection)
	return err == nil && changed
}

// replaceTableCellRegion replaces the selected rectangle with matrix contents.
// Existing cells outside the target are retained; the table grows when the
// matrix exceeds the selection. The parser projection supplies cell shape.
func (e *Editor) replaceTableCellRegion(t *leaf, region tableCellRegion, matrix *tableCellMatrix) bool {
	if t == nil || t.tab == nil || matrix == nil || t.lineStart != region.lineStart {
		return false
	}
	selection := tablemodel.Selection{
		LineStart:  region.lineStart,
		Anchor:     tablemodel.Cell{Row: region.anchorRow, Col: region.anchorCol},
		Focus:      tablemodel.Cell{Row: region.focusRow, Col: region.focusCol},
		CellRegion: true,
	}
	changed, err := e.tableController.ReplaceRegion(editorTableHost{editor: e}, selection, *matrix)
	return err == nil && changed
}
