package tableui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	tablemodel "muse/editor/blocks/table"
)

type testHost struct {
	frame   Frame
	actions []Action
}

func (h *testHost) Frame() Frame { return h.frame }
func (h *testHost) Dispatch(action Action) (Result, error) {
	h.actions = append(h.actions, action)
	return Result{Changed: true}, nil
}
func (h *testHost) EdgeAction(action Action) (Result, error) {
	h.actions = append(h.actions, action)
	return Result{Changed: true}, nil
}
func (*testHost) Focus()                                              {}
func (*testHost) MenuInput(int, ui.Rect, ui.Rect, ui.InputEvent) bool { return false }

func TestComponentExposesFrameAndDispatchesTypedActions(t *testing.T) {
	host := &testHost{frame: Frame{Tables: []Table{{LineStart: 17}}}}
	component := New(host)
	if got := component.Frame(); len(got.Tables) != 1 || got.Tables[0].LineStart != 17 {
		t.Fatalf("frame = %+v", got)
	}
	action := Action{Kind: SelectCell, LineStart: 17, Focus: tablemodel.Cell{Row: 2, Col: 1}}
	result, err := component.Dispatch(action)
	if err != nil || !result.Changed || len(host.actions) != 1 ||
		host.actions[0].Kind != action.Kind || host.actions[0].LineStart != action.LineStart || host.actions[0].Focus != action.Focus {
		t.Fatalf("dispatch = %+v, %v; actions = %+v", result, err, host.actions)
	}
}

func TestCellAtPointUsesViewportAndHorizontalOffset(t *testing.T) {
	frame := Table{
		Bounds:           Rect{X: 20, Y: 30, W: 90, H: 40},
		Viewport:         Rect{W: 60, H: 40},
		Columns:          []float32{0, 30, 60, 90},
		Rows:             []float32{0, 20, 40},
		CellCounts:       []int{3, 3},
		HorizontalOffset: 30,
	}
	cell, ok := CellAtPoint(frame, frame.Bounds.X+5, frame.Bounds.Y+5)
	if !ok || cell != (tablemodel.Cell{Row: 0, Col: 1}) {
		t.Fatalf("scrolled cell = %+v, %v", cell, ok)
	}
	if _, ok := CellAtPoint(frame, frame.Bounds.X+61, frame.Bounds.Y+5); ok {
		t.Fatal("point outside the table viewport hit a cell")
	}
}

func TestRowAndColumnAtPointClampTrailingEdges(t *testing.T) {
	rows := []float32{0, 10, 25}
	cols := []float32{0, 20, 50}
	if got := RowAtPoint(rows, 25); got != 1 {
		t.Fatalf("trailing row = %d, want 1", got)
	}
	if got := ColumnAtPoint(cols, 50); got != 1 {
		t.Fatalf("trailing column = %d, want 1", got)
	}
}
