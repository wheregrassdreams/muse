package editor

import tablemodel "github.com/wheregrassdreams/muse-editor/markdown/table"

// testTableRegion builds a structural selection through the editor domain
// model, then projects it into the component's private adapter state.
func testTableRegion(leaf *leaf, anchorRow, anchorCol, focusRow, focusCol int, right bool) *tableSelection {
	if leaf == nil || leaf.tab == nil {
		return nil
	}
	region := (tablemodel.Grid{
		LineStart: leaf.lineStart,
		Rows:      len(leaf.tab.rows),
		Cols:      leaf.tab.cols(),
	}).Region(
		tablemodel.Cell{Row: anchorRow, Col: anchorCol},
		tablemodel.Cell{Row: focusRow, Col: focusCol},
		right,
	)
	if region == nil {
		return nil
	}
	return tableSelectionFromSnapshot(region)
}
