package mdedit

import (
	"reflect"
	"testing"

	"muse/mdedit/internal/parser"
)

func TestTableCellRegionBoundsNormalizeReverseAnchor(t *testing.T) {
	h := newHarness(t, "| A | B | C |\n| :--- | :---: | ---: |\n| a | b | c |\n| d | e | f |\n| g | h | i |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	region := tableCellRegion{lineStart: table.lineStart, anchorRow: 2, anchorCol: 2, focusRow: 1, focusCol: 1}
	top, left, bottom, right := region.bounds()
	if [4]int{top, left, bottom, right} != [4]int{1, 1, 2, 2} {
		t.Fatalf("reverse bounds=%v, want [1 1 2 2]", [4]int{top, left, bottom, right})
	}
	matrix := h.e.tableCellMatrix(table, region)
	if matrix == nil {
		t.Fatal("reverse region did not produce a matrix")
	}
	want := [][]string{{"b", "c"}, {"e", "f"}}
	if !reflect.DeepEqual(matrix.cells, want) {
		t.Fatalf("reverse region cells=%v, want %v", matrix.cells, want)
	}
	wantAlign := []parser.Align{table.tab.align[1], table.tab.align[2]}
	if !reflect.DeepEqual(matrix.align, wantAlign) {
		t.Fatalf("reverse region alignment=%v, want %v", matrix.align, wantAlign)
	}
}

func TestTableCellRegionMatrixPreservesEmptyCellsAndRejectsOtherTable(t *testing.T) {
	h := newHarness(t, "| A | B |\n| --- | ---: |\n|  | x |\n| y |  |\n\n| C |\n| --- |\n| z |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	region := tableCellRegion{lineStart: table.lineStart, anchorRow: 1, anchorCol: 0, focusRow: 2, focusCol: 1}
	matrix := h.e.tableCellMatrix(table, region)
	want := [][]string{{"", "x"}, {"y", ""}}
	if matrix == nil || !reflect.DeepEqual(matrix.cells, want) {
		t.Fatalf("empty-cell matrix=%+v, want cells %v", matrix, want)
	}
	region.lineStart++
	if got := h.e.tableCellMatrix(table, region); got != nil {
		t.Fatalf("region for a different table unexpectedly resolved: %+v", got)
	}
}
