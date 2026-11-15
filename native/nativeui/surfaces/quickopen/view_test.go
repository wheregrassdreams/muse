package quickopen

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

type testRows []Row

func (rows testRows) Len() int     { return len(rows) }
func (rows testRows) At(i int) Row { return rows[i] }

func TestControlNDispatchesSelectionAction(t *testing.T) {
	state := state{Open: true}
	var actions []Action
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, ViewModel{State: state, Rows: testRows{{Path: "/a.md", Title: "A"}, {Path: "/b.md", Title: "B"}}}, func(action Action) {
			actions = append(actions, action)
			switch action.Kind {
			case QueryChanged:
				state.Query = action.Query
			case MoveSelection:
				state.Selected = action.Index
			}
		})
	}, 800, 600)
	tester.Frame()
	tester.Key(ui.Ctrl, ui.KeyN)
	tester.Frame()
	if got := lastAction(t, actions); got.Kind != MoveSelection || got.Index != 1 {
		t.Fatalf("action = %+v, want move-selection index 1", got)
	}
	tester.Type("a")
	tester.Frame()
	if got := lastAction(t, actions); got.Kind != QueryChanged || got.Query != "a" {
		t.Fatalf("action = %+v, want query-changed a", got)
	}
	tester.Key(0, ui.KeyEnter)
	tester.Frame()
	if got := lastAction(t, actions); got.Kind != OpenSelection || got.Index != 1 {
		t.Fatalf("action = %+v, want open-selection index 1", got)
	}
	tester.Key(0, ui.KeyEscape)
	tester.Frame()
	if got := lastAction(t, actions); got.Kind != Dismiss {
		t.Fatalf("action = %+v, want dismiss", got)
	}
}

func lastAction(t *testing.T, actions []Action) Action {
	t.Helper()
	if len(actions) == 0 {
		t.Fatal("no action was dispatched")
	}
	return actions[len(actions)-1]
}
