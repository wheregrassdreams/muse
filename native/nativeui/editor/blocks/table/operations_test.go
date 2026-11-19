package tableui

import (
	"strings"
	"testing"

	editormodel "github.com/wheregrassdreams/muse-editor/text/session"
	tablemodel "github.com/wheregrassdreams/muse-editor/markdown/table"
)

type operationTestSource struct{}

func (operationTestSource) TableView(int) (tablemodel.DocumentView, bool) {
	return tablemodel.DocumentView{}, false
}
func (operationTestSource) DocumentSource() string               { return "" }
func (operationTestSource) TextSelection() editormodel.Selection { return editormodel.Selection{} }
func (operationTestSource) Apply(editormodel.Transaction) error  { return nil }

func TestDispatchDocumentActionOwnsTableOperationRouting(t *testing.T) {
	controller := &tablemodel.Controller{}
	before := tablemodel.Snapshot{Mode: "Normal", Selection: &tablemodel.Selection{LineStart: 4}}
	after := tablemodel.Snapshot{Mode: "Insert", Selection: &tablemodel.Selection{LineStart: 4, Focus: tablemodel.Cell{Row: 1}}}
	if _, handled, err := DispatchDocumentAction(controller, operationTestSource{}, Action{
		Kind: RecordNavigation, BeforeState: before, AfterState: after, RowDelta: 1,
	}); err != nil || !handled {
		t.Fatalf("record navigation handled=%v err=%v", handled, err)
	}
	result, handled, err := DispatchDocumentAction(controller, operationTestSource{}, Action{
		Kind: ReplayNavigation, BeforeState: after, RowDelta: -1,
	})
	if err != nil || !handled || !result.Found || result.Snapshot.Mode != "Normal" {
		t.Fatalf("replay = %+v handled=%v err=%v", result, handled, err)
	}
	if _, handled, err := DispatchDocumentAction(controller, operationTestSource{}, Action{Kind: BeginHandleDrag}); err != nil || handled {
		t.Fatalf("UI-owned action handled=%v err=%v", handled, err)
	}
}

func TestRegisterForSelectionCopiesRowsAndSourceHeader(t *testing.T) {
	source := "| Name | Value |\n| :--- | ---: |\n| one | two |\n| three | four |"
	cell := func(value string) editormodel.TextRange {
		start := strings.Index(source, value)
		return editormodel.TextRange{Start: start, End: start + len(value)}
	}
	view := tablemodel.DocumentView{
		LineStart: 0,
		End:       len(source),
		Source:    source,
		Rows: [][]editormodel.TextRange{
			{cell("Name"), cell("Value")},
			{cell("one"), cell("two")},
			{cell("three"), cell("four")},
		},
		Alignments: []tablemodel.Alignment{tablemodel.AlignmentLeft, tablemodel.AlignmentRight},
	}
	register := RegisterForSelection(view, tablemodel.Selection{
		LineStart: 0, Axis: tablemodel.Rows, First: 2, Last: 2,
	})
	if register == nil {
		t.Fatal("expected a row register")
	}
	if got, want := register.Markdown(), "| Name | Value |\n| :--- | ---: |\n| three | four |"; got != want {
		t.Fatalf("row register = %q, want %q", got, want)
	}
}

func TestRegisterForSelectionCopiesCellRegionShape(t *testing.T) {
	source := "| a | b |\n| --- | --- |\n| c | d |"
	cell := func(value string) editormodel.TextRange {
		start := strings.Index(source, value)
		return editormodel.TextRange{Start: start, End: start + len(value)}
	}
	view := tablemodel.DocumentView{
		LineStart:  0,
		End:        len(source),
		Source:     source,
		Rows:       [][]editormodel.TextRange{{cell("a"), cell("b")}, {cell("c"), cell("d")}},
		Alignments: []tablemodel.Alignment{tablemodel.AlignmentCenter, tablemodel.AlignmentRight},
	}
	register := RegisterForSelection(view, tablemodel.Selection{
		LineStart: 0, Axis: tablemodel.Cells, CellRegion: true,
		Anchor: tablemodel.Cell{Row: 0, Col: 1}, Focus: tablemodel.Cell{Row: 1, Col: 1},
	})
	if register == nil || !register.Region() {
		t.Fatalf("cell-region register = %#v", register)
	}
	if got, want := register.Markdown(), "| b |\n| ---: |\n| d |"; got != want {
		t.Fatalf("cell-region register = %q, want %q", got, want)
	}
}

func TestPlanSelectionOperationPromotesOnlyCompleteAxes(t *testing.T) {
	grid := tablemodel.Grid{LineStart: 12, Rows: 3, Cols: 2}
	selection := tablemodel.Selection{
		LineStart: 12, Axis: tablemodel.Cells, CellRegion: true,
		Anchor: tablemodel.Cell{Row: 0, Col: 0}, Focus: tablemodel.Cell{Row: 1, Col: 1},
	}
	plan := PlanSelectionOperation(selection, grid, "d")
	if plan.ClearCellArea || plan.Selection.Axis != tablemodel.Rows || plan.Selection.First != 0 || plan.Selection.Last != 1 {
		t.Fatalf("full-row selection plan = %+v", plan)
	}
	selection.Anchor = tablemodel.Cell{Row: 0, Col: 0}
	selection.Focus = tablemodel.Cell{Row: 2, Col: 0}
	plan = PlanSelectionOperation(selection, grid, "c")
	if plan.ClearCellArea || plan.Selection.Axis != tablemodel.Columns || plan.Selection.First != 0 || plan.Selection.Last != 0 {
		t.Fatalf("full-column selection plan = %+v", plan)
	}
	selection.Focus = tablemodel.Cell{Row: 1, Col: 0}
	plan = PlanSelectionOperation(selection, grid, "d")
	if !plan.ClearCellArea || plan.Selection.Axis != tablemodel.Cells {
		t.Fatalf("partial-cell selection plan = %+v", plan)
	}
	plan = PlanSelectionOperation(selection, grid, "y")
	if plan.ClearCellArea || plan.Selection.Axis != tablemodel.Cells {
		t.Fatalf("yank plan changed cell selection: %+v", plan)
	}
}
