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

func TestContentCloseAndOpenActionAreTyped(t *testing.T) {
	opened := ""
	closed := 0
	content := NewContent(SurfaceOptions{
		Candidates: &testCandidates{byQuery: map[string]Rows{"": testRows{{Path: "/notes/a.md", Title: "A"}}}},
		OnOpen:     func(path string) { opened = path },
		OnClose:    func() { closed++ },
	})
	content.Bind(testControls{close: func() { content.Close() }})
	tester := ui.NewTester(func(c *ui.Context) { content.Render(c) }, 800, 600)
	tester.Frame()
	tester.Key(0, ui.KeyEnter)
	tester.Frame()
	if opened != "/notes/a.md" {
		t.Fatalf("OpenSelection path = %q", opened)
	}
	content.Close()
	if closed != 1 || !content.closed {
		t.Fatalf("close count=%d closed=%v", closed, content.closed)
	}
}

type testControls struct{ close func() }

func (c testControls) CloseSelf() { c.close() }
func (testControls) FocusSelf()   {}

func lastAction(t *testing.T, actions []Action) Action {
	t.Helper()
	if len(actions) == 0 {
		t.Fatal("no action was dispatched")
	}
	return actions[len(actions)-1]
}
