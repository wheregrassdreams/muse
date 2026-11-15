package table

import (
	"strings"
	"testing"

	"muse/editor/internal/core"
)

type testHost struct {
	view        DocumentView
	selection   core.Selection
	transaction core.Transaction
}

func (h *testHost) TableView(lineStart int) (DocumentView, bool) {
	return h.view, h.view.LineStart == lineStart
}

func (h *testHost) TextSelection() core.Selection { return h.selection }

func (h *testHost) DocumentSource() string { return h.view.Source }

func (h *testHost) Apply(transaction core.Transaction) error {
	if err := transaction.Validate(len(h.view.Source)); err != nil {
		return err
	}
	h.transaction = transaction
	return nil
}

func TestSnapshotCloneOwnsSelectionValues(t *testing.T) {
	snapshot := Snapshot{
		Mode:          "Visual",
		TextSelection: core.Selection{Anchor: 3, Head: 9},
		Selection:     &Selection{LineStart: 2, Anchor: Cell{Row: 0, Col: 1}, Focus: Cell{Row: 2, Col: 3}, Axis: Cells},
		Bridge:        &ContentBridge{LineStart: 2, Axis: Columns, Col: 1, Direction: 1},
	}
	clone := Clone(snapshot)
	if !Equal(snapshot, clone) {
		t.Fatal("clone changed snapshot value")
	}
	clone.Selection.Focus.Col++
	clone.Bridge.Col++
	if Equal(snapshot, clone) {
		t.Fatal("mutating cloned snapshot changed equality with original")
	}
	if snapshot.Selection.Focus.Col != 3 || snapshot.Bridge.Col != 1 {
		t.Fatal("clone shares mutable pointers with original")
	}
}

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
		Rows: [][]core.TextRange{
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
	view := DocumentView{LineStart: 0, End: len(source), Source: source, Rows: [][]core.TextRange{{{Start: 3, End: len(source) + 1}}}}
	if _, ok := CopyRegion(view, Selection{LineStart: 0, Anchor: Cell{}, Focus: Cell{}}); ok {
		t.Fatal("CopyRegion accepted an out-of-range parser cell span")
	}
}

func TestControllerClearRegionUsesOneAtomicHostTransaction(t *testing.T) {
	source := "| aa | bb |\n| --- | --- |\n| cc | dd |\n"
	host := &testHost{
		view: DocumentView{
			LineStart: 0, End: len(source), Source: source,
			Rows: [][]core.TextRange{
				{{Start: 2, End: 4}, {Start: 7, End: 9}},
				{{Start: 27, End: 29}, {Start: 32, End: 34}},
			},
		},
		selection: core.Selection{Anchor: 27, Head: 34},
	}
	controller := &Controller{}
	changed, err := controller.ClearRegion(host, Selection{
		LineStart: 0, Anchor: Cell{Row: 1, Col: 0}, Focus: Cell{Row: 1, Col: 1}, CellRegion: true,
	})
	if err != nil || !changed {
		t.Fatalf("ClearRegion() = %v, %v", changed, err)
	}
	tx := host.transaction
	if len(tx.Edits) != 2 || tx.Edits[0].Range != (core.TextRange{Start: 27, End: 29}) || tx.Edits[1].Range != (core.TextRange{Start: 32, End: 34}) {
		t.Fatalf("transaction edits = %+v", tx.Edits)
	}
	if tx.Before != host.selection || tx.After != (core.Selection{Anchor: 27, Head: 27}) {
		t.Fatalf("transaction selection = before %+v after %+v", tx.Before, tx.After)
	}
}

func TestControllerReplaceRegionSerializesAndGrowsTableAtomically(t *testing.T) {
	source := "| A | B |\n| --- | --- |\n| x | y |\n"
	host := &testHost{
		view: DocumentView{
			LineStart: 0, End: len(source) - 1, Source: source,
			Rows: [][]core.TextRange{
				{{Start: 2, End: 3}, {Start: 6, End: 7}},
				{{Start: 26, End: 27}, {Start: 30, End: 31}},
			},
		},
		selection: core.Selection{Anchor: 26, Head: 31},
	}
	controller := &Controller{}
	changed, err := controller.ReplaceRegion(host,
		Selection{LineStart: 0, Anchor: Cell{Row: 1, Col: 1}, Focus: Cell{Row: 1, Col: 1}, CellRegion: true},
		Matrix{Cells: [][]string{{"new", "extra"}}, Alignments: []Alignment{AlignmentCenter, AlignmentRight}},
	)
	if err != nil || !changed {
		t.Fatalf("ReplaceRegion() = %v, %v", changed, err)
	}
	tx := host.transaction
	if len(tx.Edits) != 1 || tx.Edits[0].Range != (core.TextRange{Start: 0, End: len(source) - 1}) {
		t.Fatalf("replacement transaction = %+v", tx)
	}
	want := "| A | B |  |\n| --- | :---: | ---: |\n| x | new | extra |"
	if tx.Edits[0].Text != want {
		t.Fatalf("serialized table = %q, want %q", tx.Edits[0].Text, want)
	}
}

func TestControllerAddColumnUsesParserPipeOffsetsAndPreservesSelection(t *testing.T) {
	source := "| a | b |\n| --- | --- |\n| c | d |"
	lines := []LineView{
		{Range: core.TextRange{Start: 0, End: 9}, Pipes: []int{0, 4, 8}},
		{Range: core.TextRange{Start: 10, End: 23}, Pipes: []int{10, 16, 22}},
		{Range: core.TextRange{Start: 24, End: 33}, Pipes: []int{24, 28, 32}},
	}
	host := &testHost{
		view:      DocumentView{LineStart: 0, End: len(source), Source: source, Lines: lines},
		selection: core.Selection{Anchor: 4, Head: 4},
	}
	changed, err := (&Controller{}).AddColumnRight(host, 0)
	if err != nil || !changed {
		t.Fatalf("AddColumnRight() = %v, %v", changed, err)
	}
	tx := host.transaction
	if len(tx.Edits) != 3 || tx.Edits[0].Range.Start != 9 || tx.Edits[1].Range.Start != 23 || tx.Edits[2].Range.Start != 33 {
		t.Fatalf("column insertion edits = %+v", tx.Edits)
	}
	if tx.Edits[1].Text != " --- |" || tx.Edits[0].Text != "   |" || tx.Edits[2].Text != "   |" {
		t.Fatalf("column insertion text = %+v", tx.Edits)
	}
	if tx.After != (core.Selection{Anchor: 4, Head: 4}) {
		t.Fatalf("selection moved from unrelated source: %+v", tx.After)
	}
}

func TestControllerAddRowUsesContinuationPrefixAndMapsSelection(t *testing.T) {
	source := "| a | b |\n| --- | --- |\n| c | d |"
	host := &testHost{
		view: DocumentView{
			LineStart: 0, End: len(source), Source: source, ContinuationPrefix: "> ",
			Rows:  [][]core.TextRange{{{Start: 2, End: 3}, {Start: 6, End: 7}}, {{Start: 26, End: 27}, {Start: 30, End: 31}}},
			Lines: []LineView{{Range: core.TextRange{Start: 0, End: 9}}, {Range: core.TextRange{Start: 10, End: 23}}, {Range: core.TextRange{Start: 24, End: 33}}},
		},
		selection: core.Selection{Anchor: len(source), Head: len(source)},
	}
	changed, err := (&Controller{}).AddRowBelow(host, 0)
	if err != nil || !changed {
		t.Fatalf("AddRowBelow() = %v, %v", changed, err)
	}
	tx := host.transaction
	if len(tx.Edits) != 1 || tx.Edits[0].Text != "\n> |   |   |" {
		t.Fatalf("row insertion = %+v", tx.Edits)
	}
	if tx.After.Anchor != len(source)+len(tx.Edits[0].Text) || tx.After.Head != tx.After.Anchor {
		t.Fatalf("selection after append = %+v", tx.After)
	}
}

func TestControllerReorderRowsAndColumnsSerializesThroughHost(t *testing.T) {
	source := "| h1 | h2 |\n| :--- | ---: |\n| r1 | x |\n| r2 | y |"
	rangeFor := func(value string) core.TextRange {
		start := strings.Index(source, value)
		return core.TextRange{Start: start, End: start + len(value)}
	}
	view := DocumentView{
		LineStart: 0, End: len(source), Source: source, Alignments: []Alignment{AlignmentLeft, AlignmentRight},
		Rows: [][]core.TextRange{
			{rangeFor("h1"), rangeFor("h2")},
			{rangeFor("r1"), rangeFor("x")},
			{rangeFor("r2"), rangeFor("y")},
		},
	}
	host := &testHost{view: view, selection: core.Selection{Anchor: 0, Head: 0}}
	changed, insertion, err := (&Controller{}).Reorder(host, 0, Rows, 1, 1, 3)
	if err != nil || !changed || insertion != 2 {
		t.Fatalf("row Reorder() = %v, %d, %v", changed, insertion, err)
	}
	wantRows := "| h1 | h2 |\n| :--- | ---: |\n| r2 | y |\n| r1 | x |"
	if got := host.transaction.Edits[0].Text; got != wantRows {
		t.Fatalf("row reorder text = %q, want %q", got, wantRows)
	}
	host.transaction = core.Transaction{}
	changed, insertion, err = (&Controller{}).Reorder(host, 0, Columns, 0, 0, 2)
	if err != nil || !changed || insertion != 1 {
		t.Fatalf("column Reorder() = %v, %d, %v", changed, insertion, err)
	}
	wantColumns := "| h2 | h1 |\n| ---: | :--- |\n| x | r1 |\n| y | r2 |"
	if got := host.transaction.Edits[0].Text; got != wantColumns {
		t.Fatalf("column reorder text = %q, want %q", got, wantColumns)
	}
}

func TestControllerDeleteRowsCreatesSingleNormalTransaction(t *testing.T) {
	source := "| h |\n| --- |\n| a |\n| b |"
	host := &testHost{
		view: DocumentView{
			LineStart: 0, End: len(source), Source: source,
			Rows:  [][]core.TextRange{{{Start: 2, End: 3}}, {{Start: 16, End: 17}}, {{Start: 22, End: 23}}},
			Lines: []LineView{{Range: core.TextRange{Start: 0, End: 5}}, {Range: core.TextRange{Start: 6, End: 13}}, {Range: core.TextRange{Start: 14, End: 19}}, {Range: core.TextRange{Start: 20, End: 25}}},
		},
		selection: core.Selection{Anchor: 14, Head: 18},
	}
	changed, caret, err := (&Controller{}).DeleteRows(host, 0, 1, 1, false, false)
	if err != nil || !changed || caret != 14 {
		t.Fatalf("DeleteRows() = %v, %d, %v", changed, caret, err)
	}
	tx := host.transaction
	if len(tx.Edits) != 1 || tx.Edits[0].Range != (core.TextRange{Start: 14, End: 20}) || tx.Edits[0].Text != "" {
		t.Fatalf("delete transaction = %+v", tx)
	}
}

func TestControllerDeleteColumnsPreservesRowsAndAlignment(t *testing.T) {
	source := "| A | B | C |\n| :--- | :---: | ---: |\n| one | two | three |"
	lineView := func(line string) LineView {
		start := strings.Index(source, line)
		pipes := make([]int, 0, strings.Count(line, "|"))
		for i, r := range line {
			if r == '|' {
				pipes = append(pipes, start+i)
			}
		}
		return LineView{Range: core.TextRange{Start: start, End: start + len(line)}, Pipes: pipes}
	}
	header := "| A | B | C |"
	delimiter := "| :--- | :---: | ---: |"
	body := "| one | two | three |"
	cell := func(text string) core.TextRange {
		start := strings.Index(source, text)
		return core.TextRange{Start: start, End: start + len(text)}
	}
	view := DocumentView{
		LineStart: 0, End: len(source), Source: source,
		Rows:       [][]core.TextRange{{cell("A"), cell("B"), cell("C")}, {cell("one"), cell("two"), cell("three")}},
		Lines:      []LineView{lineView(header), lineView(delimiter), lineView(body)},
		Alignments: []Alignment{AlignmentLeft, AlignmentCenter, AlignmentRight},
	}
	for _, tc := range []struct {
		name   string
		change bool
		want   string
	}{
		{"delete", false, "| A | C |\n| :-- | --: |\n| one | three |"},
		{"clear", true, "| A |  | C |\n| :-- | :-: | --: |\n| one |  | three |"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := &testHost{view: view, selection: core.Selection{Anchor: 0, Head: 0}}
			changed, _, err := (&Controller{}).DeleteColumns(host, 0, 1, 1, tc.change)
			if err != nil || !changed {
				t.Fatalf("DeleteColumns() = %v, %v", changed, err)
			}
			if got := host.transaction.Edits[0].Text; got != tc.want {
				t.Fatalf("column edit text = %q, want %q", got, tc.want)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
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
