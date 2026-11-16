package editor

import (
	"reflect"
	"testing"

	tablemodel "muse/editor/blocks/table"
)

func TestTableCellRegionBoundsNormalizeReverseAnchor(t *testing.T) {
	h := newHarness(t, "| A | B | C |\n| :--- | :---: | ---: |\n| a | b | c |\n| d | e | f |\n| g | h | i |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	region := tablemodel.Selection{LineStart: table.lineStart, Anchor: tablemodel.Cell{Row: 2, Col: 2}, Focus: tablemodel.Cell{Row: 1, Col: 1}, CellRegion: true}
	top, left, bottom, right := tableRegionBounds(region)
	if [4]int{top, left, bottom, right} != [4]int{1, 1, 2, 2} {
		t.Fatalf("reverse bounds=%v, want [1 1 2 2]", [4]int{top, left, bottom, right})
	}
	matrix := h.e.tableCellMatrix(table, region)
	if matrix == nil {
		t.Fatal("reverse region did not produce a matrix")
	}
	want := [][]string{{"b", "c"}, {"e", "f"}}
	if !reflect.DeepEqual(matrix.Cells, want) {
		t.Fatalf("reverse region cells=%v, want %v", matrix.Cells, want)
	}
	wantAlign := []tablemodel.Alignment{tablemodel.Alignment(table.tab.align[1]), tablemodel.Alignment(table.tab.align[2])}
	if !reflect.DeepEqual(matrix.Alignments, wantAlign) {
		t.Fatalf("reverse region alignment=%v, want %v", matrix.Alignments, wantAlign)
	}
}

func TestTableCellRegionMatrixPreservesEmptyCellsAndRejectsOtherTable(t *testing.T) {
	h := newHarness(t, "| A | B |\n| --- | ---: |\n|  | x |\n| y |  |\n\n| C |\n| --- |\n| z |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	region := tablemodel.Selection{LineStart: table.lineStart, Anchor: tablemodel.Cell{Row: 1, Col: 0}, Focus: tablemodel.Cell{Row: 2, Col: 1}, CellRegion: true}
	matrix := h.e.tableCellMatrix(table, region)
	want := [][]string{{"", "x"}, {"y", ""}}
	if matrix == nil || !reflect.DeepEqual(matrix.Cells, want) {
		t.Fatalf("empty-cell matrix=%+v, want cells %v", matrix, want)
	}
	region.LineStart++
	if got := h.e.tableCellMatrix(table, region); got != nil {
		t.Fatalf("region for a different table unexpectedly resolved: %+v", got)
	}
}
