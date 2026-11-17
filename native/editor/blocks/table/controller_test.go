package table

import (
	"strings"
	"testing"

	"muse/editor"
)

func TestControllerClearRegionUsesOneAtomicHostTransaction(t *testing.T) {
	source := "| aa | bb |\n| --- | --- |\n| cc | dd |\n"
	host := &testHost{
		view: DocumentView{
			LineStart: 0, End: len(source), Source: source,
			Rows: [][]editor.TextRange{
				{{Start: 2, End: 4}, {Start: 7, End: 9}},
				{{Start: 27, End: 29}, {Start: 32, End: 34}},
			},
		},
		selection: editor.Selection{Anchor: 27, Head: 34},
	}
	controller := &Controller{}
	changed, err := controller.ClearRegion(host, Selection{
		LineStart: 0, Anchor: Cell{Row: 1, Col: 0}, Focus: Cell{Row: 1, Col: 1}, CellRegion: true,
	})
	if err != nil || !changed {
		t.Fatalf("ClearRegion() = %v, %v", changed, err)
	}
	tx := host.transaction
	if len(tx.Edits) != 2 || tx.Edits[0].Range != (editor.TextRange{Start: 27, End: 29}) || tx.Edits[1].Range != (editor.TextRange{Start: 32, End: 34}) {
		t.Fatalf("transaction edits = %+v", tx.Edits)
	}
	if tx.Before != host.selection || tx.After != (editor.Selection{Anchor: 27, Head: 27}) {
		t.Fatalf("transaction selection = before %+v after %+v", tx.Before, tx.After)
	}
}

func TestControllerReplaceRegionSerializesAndGrowsTableAtomically(t *testing.T) {
	source := "| A | B |\n| --- | --- |\n| x | y |\n"
	host := &testHost{
		view: DocumentView{
			LineStart: 0, End: len(source) - 1, Source: source,
			Rows: [][]editor.TextRange{
				{{Start: 2, End: 3}, {Start: 6, End: 7}},
				{{Start: 26, End: 27}, {Start: 30, End: 31}},
			},
		},
		selection: editor.Selection{Anchor: 26, Head: 31},
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
	if len(tx.Edits) != 1 || tx.Edits[0].Range != (editor.TextRange{Start: 0, End: len(source) - 1}) {
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
		{Range: editor.TextRange{Start: 0, End: 9}, Pipes: []int{0, 4, 8}},
		{Range: editor.TextRange{Start: 10, End: 23}, Pipes: []int{10, 16, 22}},
		{Range: editor.TextRange{Start: 24, End: 33}, Pipes: []int{24, 28, 32}},
	}
	host := &testHost{
		view:      DocumentView{LineStart: 0, End: len(source), Source: source, Lines: lines},
		selection: editor.Selection{Anchor: 4, Head: 4},
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
	if tx.After != (editor.Selection{Anchor: 4, Head: 4}) {
		t.Fatalf("selection moved from unrelated source: %+v", tx.After)
	}
}

func TestControllerAddRowUsesContinuationPrefixAndMapsSelection(t *testing.T) {
	source := "| a | b |\n| --- | --- |\n| c | d |"
	host := &testHost{
		view: DocumentView{
			LineStart: 0, End: len(source), Source: source, ContinuationPrefix: "> ",
			Rows:  [][]editor.TextRange{{{Start: 2, End: 3}, {Start: 6, End: 7}}, {{Start: 26, End: 27}, {Start: 30, End: 31}}},
			Lines: []LineView{{Range: editor.TextRange{Start: 0, End: 9}}, {Range: editor.TextRange{Start: 10, End: 23}}, {Range: editor.TextRange{Start: 24, End: 33}}},
		},
		selection: editor.Selection{Anchor: len(source), Head: len(source)},
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
	rangeFor := func(value string) editor.TextRange {
		start := strings.Index(source, value)
		return editor.TextRange{Start: start, End: start + len(value)}
	}
	view := DocumentView{
		LineStart: 0, End: len(source), Source: source, Alignments: []Alignment{AlignmentLeft, AlignmentRight},
		Rows: [][]editor.TextRange{
			{rangeFor("h1"), rangeFor("h2")},
			{rangeFor("r1"), rangeFor("x")},
			{rangeFor("r2"), rangeFor("y")},
		},
	}
	host := &testHost{view: view, selection: editor.Selection{Anchor: 0, Head: 0}}
	changed, insertion, err := (&Controller{}).Reorder(host, 0, Rows, 1, 1, 3)
	if err != nil || !changed || insertion != 2 {
		t.Fatalf("row Reorder() = %v, %d, %v", changed, insertion, err)
	}
	wantRows := "| h1 | h2 |\n| :--- | ---: |\n| r2 | y |\n| r1 | x |"
	if got := host.transaction.Edits[0].Text; got != wantRows {
		t.Fatalf("row reorder text = %q, want %q", got, wantRows)
	}
	host.transaction = editor.Transaction{}
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
			Rows:  [][]editor.TextRange{{{Start: 2, End: 3}}, {{Start: 16, End: 17}}, {{Start: 22, End: 23}}},
			Lines: []LineView{{Range: editor.TextRange{Start: 0, End: 5}}, {Range: editor.TextRange{Start: 6, End: 13}}, {Range: editor.TextRange{Start: 14, End: 19}}, {Range: editor.TextRange{Start: 20, End: 25}}},
		},
		selection: editor.Selection{Anchor: 14, Head: 18},
	}
	changed, caret, err := (&Controller{}).DeleteRows(host, 0, 1, 1, false, false)
	if err != nil || !changed || caret != 14 {
		t.Fatalf("DeleteRows() = %v, %d, %v", changed, caret, err)
	}
	tx := host.transaction
	if len(tx.Edits) != 1 || tx.Edits[0].Range != (editor.TextRange{Start: 14, End: 20}) || tx.Edits[0].Text != "" {
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
		return LineView{Range: editor.TextRange{Start: start, End: start + len(line)}, Pipes: pipes}
	}
	header := "| A | B | C |"
	delimiter := "| :--- | :---: | ---: |"
	body := "| one | two | three |"
	cell := func(text string) editor.TextRange {
		start := strings.Index(source, text)
		return editor.TextRange{Start: start, End: start + len(text)}
	}
	view := DocumentView{
		LineStart: 0, End: len(source), Source: source,
		Rows:       [][]editor.TextRange{{cell("A"), cell("B"), cell("C")}, {cell("one"), cell("two"), cell("three")}},
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
			host := &testHost{view: view, selection: editor.Selection{Anchor: 0, Head: 0}}
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
