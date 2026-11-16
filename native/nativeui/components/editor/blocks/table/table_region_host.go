package editor

import (
	tablemodel "muse/editor/blocks/table"
	syntax "muse/markdown/syntax"
	tableui "muse/nativeui/components/editor/blocks/table"
)

type tableCellMatrix = tablemodel.Matrix

// tableRegionBounds projects the canonical cell endpoints into a rectangle
// for leaf geometry and selection painting. Selection semantics remain owned
// by editor/blocks/table.
func tableRegionBounds(selection tablemodel.Selection) (top, left, bottom, right int) {
	top, bottom = selection.Anchor.Row, selection.Focus.Row
	left, right = selection.Anchor.Col, selection.Focus.Col
	if top > bottom {
		top, bottom = bottom, top
	}
	if left > right {
		left, right = right, left
	}
	return
}

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

func (e *Editor) tableCellMatrix(t *leaf, selection tablemodel.Selection) *tableCellMatrix {
	if t == nil || t.tab == nil || t.lineStart != selection.LineStart {
		return nil
	}
	grid := tablemodel.Grid{LineStart: t.lineStart, Rows: len(t.tab.rows), Cols: t.tab.cols()}
	selection, ok := selection.Clamp(grid)
	if !ok {
		return nil
	}
	view := documentViewForTable(e, t)
	matrix, ok := tablemodel.CopyRegion(view, selection)
	if !ok {
		return nil
	}
	_, left, _, right := tableRegionBounds(selection)
	matrix.Alignments = make([]tablemodel.Alignment, right-left+1)
	for col := left; col <= right; col++ {
		matrix.Alignments[col-left] = tablemodel.Alignment(alignAt(t.tab.align, col))
	}
	return &matrix
}

func (e *Editor) clearTableCellRegion(t *leaf, selection tablemodel.Selection) bool {
	if t == nil || t.tab == nil || t.lineStart != selection.LineStart {
		return false
	}
	result, err := e.tableUI().Dispatch(tableui.Action{
		Kind: tableui.ClearRegion, LineStart: selection.LineStart, Selection: &selection,
	})
	return err == nil && result.Changed
}

func (e *Editor) replaceTableCellRegion(t *leaf, selection tablemodel.Selection, matrix *tableCellMatrix) bool {
	if t == nil || t.tab == nil || matrix == nil || t.lineStart != selection.LineStart {
		return false
	}
	result, err := e.tableUI().Dispatch(tableui.Action{
		Kind: tableui.ReplaceRegion, LineStart: selection.LineStart, Selection: &selection, Matrix: *matrix,
	})
	return err == nil && result.Changed
}
