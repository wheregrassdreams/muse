package table

import (
	"strings"
	"testing"

	"muse/editor"
)

func testDocumentView(source string) DocumentView {
	start := strings.Index(source, "| A | B |")
	end := strings.Index(source[start:], "\n") + start
	end = strings.Index(source[end+1:], "\n") + end + 1
	end = strings.Index(source[end+1:], "\n") + end + 1
	lineStarts := []int{start}
	for at := start; at < end; {
		next := strings.IndexByte(source[at:], '\n')
		if next < 0 {
			break
		}
		at += next + 1
		if at < end {
			lineStarts = append(lineStarts, at)
		}
	}
	lines := make([]LineView, 0, len(lineStarts))
	for _, lineStart := range lineStarts {
		lineEnd := strings.IndexByte(source[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(source) - lineStart
		}
		lineEnd += lineStart
		var pipes []int
		for at := lineStart; at < lineEnd; at++ {
			if source[at] == '|' {
				pipes = append(pipes, at)
			}
		}
		lines = append(lines, LineView{Range: editor.TextRange{Start: lineStart, End: lineEnd}, Pipes: pipes})
	}
	cell := func(value string) editor.TextRange {
		at := strings.Index(source[start:end], value) + start
		return editor.TextRange{Start: at, End: at + len(value)}
	}
	return DocumentView{
		LineStart: start, End: end, Source: source,
		Rows:  [][]editor.TextRange{{cell("A"), cell("B")}, {cell("x"), cell("y")}},
		Lines: lines,
	}
}

func TestCellAtSourceOffsetUsesParserLineProjectionForDelimiters(t *testing.T) {
	source := "before\n| A | B |\n| --- | --- |\n| x | y |\nafter"
	view := testDocumentView(source)
	cases := []struct {
		name string
		off  int
		want Cell
	}{
		{"leading whitespace", view.LineStart + 1, Cell{Row: 0, Col: 0}},
		{"inter-cell delimiter", view.LineStart + 4, Cell{Row: 0, Col: 1}},
		{"separator row", view.Lines[1].Range.Start + 4, Cell{Row: 0, Col: 0}},
		{"body whitespace", view.Lines[2].Range.Start + 1, Cell{Row: 1, Col: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := CellAtSourceOffset(view, tc.off, PreferNextCell)
			if !ok || got != tc.want {
				t.Fatalf("CellAtSourceOffset(%d) = %+v, %v; want %+v", tc.off, got, ok, tc.want)
			}
		})
	}
}

func TestSelectionForTextRangeKeepsInnerTextAndPromotesCrossingRanges(t *testing.T) {
	source := "before\n| A | B |\n| --- | --- |\n| x | y |\nafter"
	view := testDocumentView(source)
	innerStart := view.Rows[0][0].Start
	if _, ok := SelectionForTextRange(view, editor.Selection{Anchor: innerStart, Head: innerStart + 1}); ok {
		t.Fatal("selection wholly inside the table was promoted")
	}
	region, ok := SelectionForTextRange(view, editor.Selection{Anchor: 0, Head: view.Rows[0][1].Start + 1})
	if !ok || region.Axis != Rows || region.Anchor != (Cell{Row: 0, Col: 0}) || region.Focus != (Cell{Row: 0, Col: 1}) {
		t.Fatalf("crossing range region = %+v, %v", region, ok)
	}
	whole, ok := SelectionForTextRange(view, editor.Selection{Anchor: view.LineStart, Head: view.End})
	if !ok || whole.Axis != Whole {
		t.Fatalf("whole-table range = %+v, %v", whole, ok)
	}
}

func TestSelectionProjectsToSourceAndContentRanges(t *testing.T) {
	source := "before\n| A | B |\n| --- | --- |\n| x | y |\nafter"
	view := testDocumentView(source)
	grid := Grid{LineStart: view.LineStart, Rows: len(view.Rows), Cols: 2}
	cases := []struct {
		name      string
		selection *Selection
		source    editor.TextRange
		content   editor.TextRange
	}{
		{"row", grid.AxisRegion(Rows, 1, 1, true), view.Lines[2].Range, editor.TextRange{Start: view.Rows[1][0].Start, End: view.Rows[1][1].End}},
		{"column", grid.AxisRegion(Columns, 1, 1, true), editor.TextRange{Start: view.Rows[0][1].Start, End: view.Rows[1][1].End}, editor.TextRange{Start: view.Rows[0][1].Start, End: view.Rows[1][1].End}},
		{"whole", grid.AxisRegion(Whole, 0, 0, true), editor.TextRange{Start: view.LineStart, End: view.End}, editor.TextRange{Start: view.LineStart, End: view.End}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.selection == nil {
				t.Fatal("AxisRegion returned nil")
			}
			gotSource, ok := tc.selection.SourceRange(view)
			if !ok || gotSource != tc.source {
				t.Fatalf("SourceRange() = %+v, %v; want %+v", gotSource, ok, tc.source)
			}
			gotContent, ok := tc.selection.ContentRange(view)
			if !ok || gotContent != tc.content {
				t.Fatalf("ContentRange() = %+v, %v; want %+v", gotContent, ok, tc.content)
			}
		})
	}
}

func TestProjectDocumentSelectionPreservesTextAroundStructuralTables(t *testing.T) {
	source := "before\n| A | B |\n| --- | --- |\n| x | y |\nafter"
	view := testDocumentView(source)
	parts := ProjectDocumentSelection(editor.Selection{Anchor: 0, Head: len(source)}, []DocumentView{view})
	if len(parts) != 3 {
		t.Fatalf("ProjectDocumentSelection parts = %+v", parts)
	}
	if parts[0].Source != (editor.TextRange{Start: 0, End: view.LineStart}) || parts[0].Table != nil {
		t.Fatalf("prefix part = %+v", parts[0])
	}
	if parts[1].Source != (editor.TextRange{Start: view.LineStart, End: view.End}) || parts[1].Table == nil || parts[1].Table.Axis != Whole {
		t.Fatalf("table part = %+v", parts[1])
	}
	if parts[2].Source != (editor.TextRange{Start: view.End, End: len(source)}) || parts[2].Table != nil {
		t.Fatalf("suffix part = %+v", parts[2])
	}
}

func TestRegionSelectionTransitionsKeepTypedOriginAndReportBoundary(t *testing.T) {
	grid := Grid{LineStart: 10, Rows: 2, Cols: 2}
	selection, ok := PromoteCellRegion(grid, Cell{Row: 0, Col: 0}, Cell{Row: 0, Col: 1}, true, SelectionOriginVisualText)
	if !ok || selection.Origin != SelectionOriginVisualText || selection.Axis != Rows {
		t.Fatalf("PromoteCellRegion() = %+v, %v", selection, ok)
	}
	moved := MoveRegion(grid, *selection, 1, 0)
	if !moved.Handled || moved.Exited || !moved.Changed || moved.Selection.Anchor != selection.Anchor || moved.Selection.Focus != (Cell{Row: 1, Col: 1}) || moved.Selection.Axis != Whole || moved.Selection.Origin != SelectionOriginVisualText {
		t.Fatalf("within-grid MoveRegion() = %+v", moved)
	}
	edge := MoveRegion(grid, *selection, 0, 1)
	if !edge.Handled || !edge.Exited || edge.ExitCell != (Cell{Row: 0, Col: 1}) || edge.Direction != (Cell{Col: 1}) {
		t.Fatalf("edge MoveRegion() = %+v", edge)
	}
}
