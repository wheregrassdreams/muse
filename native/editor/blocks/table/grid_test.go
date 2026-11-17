package table

import (
	"testing"

	"muse/editor"
)

func TestGridRegionInfersWholeRowsColumnsAndCellRectangles(t *testing.T) {
	grid := Grid{LineStart: 12, Rows: 4, Cols: 3}
	cases := []struct {
		name          string
		anchor, focus Cell
		axis          Axis
		first, last   int
	}{
		{"whole", Cell{0, 0}, Cell{3, 2}, Whole, 0, 3},
		{"rows", Cell{1, 0}, Cell{2, 2}, Rows, 1, 2},
		{"columns", Cell{0, 1}, Cell{3, 2}, Columns, 1, 2},
		{"rectangle", Cell{1, 0}, Cell{2, 1}, Cells, 1, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := grid.Region(tc.anchor, tc.focus, true)
			if got == nil || got.LineStart != grid.LineStart || got.Axis != tc.axis || got.First != tc.first || got.Last != tc.last || !got.CellRegion || !got.Right {
				t.Fatalf("Region(%+v, %+v) = %+v", tc.anchor, tc.focus, got)
			}
		})
	}
}

func TestGridRegionClampsEndpoints(t *testing.T) {
	got := (Grid{LineStart: 8, Rows: 2, Cols: 3}).Region(Cell{-1, 9}, Cell{1, 1}, false)
	if got == nil || got.Anchor != (Cell{0, 2}) || got.Focus != (Cell{1, 1}) || got.Right {
		t.Fatalf("Region clamped to %+v", got)
	}
}

func TestAxisRegionAndContainsUseSemanticSelection(t *testing.T) {
	grid := Grid{LineStart: 4, Rows: 3, Cols: 4}
	rows := grid.AxisRegion(Rows, 2, 0, false)
	if rows == nil || rows.Axis != Rows || rows.First != 0 || rows.Last != 2 || rows.Anchor.Row != 2 || rows.Focus.Row != 0 || rows.Right {
		t.Fatalf("row selection = %+v", rows)
	}
	if !rows.Contains(Cell{Row: 1, Col: 3}) || rows.Contains(Cell{Row: 3, Col: 0}) {
		t.Fatal("row membership did not follow the semantic row range")
	}
	columns := grid.AxisRegion(Columns, 1, 2, true)
	if columns == nil || columns.Axis != Columns || columns.First != 1 || columns.Last != 2 || !columns.Contains(Cell{Row: 0, Col: 2}) || columns.Contains(Cell{Row: 2, Col: 3}) {
		t.Fatalf("column selection = %+v", columns)
	}
	cells := *grid.Region(Cell{Row: 2, Col: 3}, Cell{Row: 1, Col: 1}, true)
	top, left, bottom, right := cells.Bounds()
	if top != 1 || left != 1 || bottom != 2 || right != 3 {
		t.Fatalf("cell bounds = (%d, %d, %d, %d)", top, left, bottom, right)
	}
	if !cells.Contains(Cell{Row: 1, Col: 2}) || cells.Contains(Cell{Row: 0, Col: 2}) {
		t.Fatal("cell membership did not follow rectangular bounds")
	}
}

func TestMoveOrderUsesOriginalGridGap(t *testing.T) {
	cases := []struct {
		name     string
		count    int
		first    int
		last     int
		gap      int
		want     []int
		insertAt int
		ok       bool
	}{
		{"move earlier", 5, 3, 4, 1, []int{0, 3, 4, 1, 2}, 1, true},
		{"move later", 5, 1, 2, 5, []int{0, 3, 4, 1, 2}, 3, true},
		{"drop inside group", 5, 1, 2, 2, nil, 1, false},
		{"drop at group trailing edge", 5, 1, 2, 3, nil, 1, false},
		{"invalid range", 5, 3, 2, 0, nil, 3, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, at, ok := MoveOrder(tc.count, tc.first, tc.last, tc.gap)
			if ok != tc.ok || at != tc.insertAt || !equalInts(got, tc.want) {
				t.Fatalf("MoveOrder() = %v, %d, %v; want %v, %d, %v", got, at, ok, tc.want, tc.insertAt, tc.ok)
			}
		})
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCopyRegionReadsSourceViewAndPadsRaggedRows(t *testing.T) {
	source := "| a | bb |\n| --- | --- |\n| c |\n"
	view := DocumentView{
		LineStart: 0, End: len(source), Source: source,
		Rows: [][]editor.TextRange{
			{{Start: 2, End: 3}, {Start: 6, End: 8}},
			{{Start: 27, End: 28}},
		},
	}
	region := Selection{LineStart: 0, Anchor: Cell{0, 0}, Focus: Cell{1, 1}, CellRegion: true}
	matrix, ok := CopyRegion(view, region)
	if !ok {
		t.Fatal("CopyRegion rejected valid source view")
	}
	want := [][]string{{"a", "bb"}, {"c", ""}}
	if len(matrix.Cells) != len(want) {
		t.Fatalf("CopyRegion returned %v", matrix.Cells)
	}
	for row := range want {
		if !equalStrings(matrix.Cells[row], want[row]) {
			t.Fatalf("CopyRegion row %d = %v, want %v", row, matrix.Cells[row], want[row])
		}
	}
}

func TestCopyRegionRejectsCellRangesOutsideDocumentView(t *testing.T) {
	source := "| a |\n"
	view := DocumentView{LineStart: 0, End: len(source), Source: source, Rows: [][]editor.TextRange{{{Start: 3, End: len(source) + 1}}}}
	if _, ok := CopyRegion(view, Selection{LineStart: 0, Anchor: Cell{}, Focus: Cell{}}); ok {
		t.Fatal("CopyRegion accepted an out-of-range parser cell span")
	}
}
