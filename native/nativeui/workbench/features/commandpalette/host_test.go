package commandpalette

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestHostProjectsCandidatesAndReturnsOnlyTypedRootActions(t *testing.T) {
	host := NewHost()
	commands := []Command{
		{ID: "document.new", Name: "New Document"},
		{ID: "document.save", Name: "Save"},
		{ID: "workspace.search", Name: "Search Workspace"},
	}
	host.Open(commands)
	if !host.IsOpen() {
		t.Fatal("opening host did not open the palette")
	}

	if action := host.Apply(Action{Kind: QueryChanged, Query: "save"}); action.Kind != 0 {
		t.Fatalf("query change escaped to root as %+v", action)
	}
	model := host.ViewModel()
	if model.Query != "save" || len(model.Commands) != 1 || model.Commands[0].ID != "document.save" || model.Selected != 0 {
		t.Fatalf("filtered model = %+v", model)
	}

	if action := host.Apply(Action{Kind: Execute, ID: "workspace.search"}); action.Kind != 0 {
		t.Fatalf("invisible candidate escaped as %+v", action)
	}
	if !host.IsOpen() {
		t.Fatal("rejected stale execute closed the palette")
	}

	action := host.Apply(Action{Kind: Execute, ID: "document.save"})
	if action.Kind != Execute || action.ID != "document.save" || !actionIsOnlyID(action) {
		t.Fatalf("execute outcome = %+v, want typed command ID only", action)
	}
	if host.IsOpen() {
		t.Fatal("accepted execute did not close the palette")
	}
}

func TestHostOpenResetsQueryAndDismissReturnsTypedIntent(t *testing.T) {
	host := NewHost()
	host.Open([]Command{{ID: "one", Name: "One"}})
	host.Apply(Action{Kind: QueryChanged, Query: "missing"})
	if got := host.ViewModel().Query; got != "missing" {
		t.Fatalf("query = %q, want missing", got)
	}

	host.Open([]Command{{ID: "two", Name: "Two"}})
	if model := host.ViewModel(); model.Query != "" || len(model.Commands) != 1 || model.Commands[0].ID != "two" {
		t.Fatalf("reopened model = %+v", model)
	}
	action := host.Apply(Action{Kind: Dismiss})
	if action.Kind != Dismiss || action.ID != "" || action.Query != "" || action.Index != 0 {
		t.Fatalf("dismiss outcome = %+v", action)
	}
	if host.IsOpen() {
		t.Fatal("dismiss did not close the palette")
	}
}

func TestHostRenderReturnsTypedExecuteAndDismissActions(t *testing.T) {
	host := NewHost()
	host.Open([]Command{
		{ID: "document.new", Name: "New Document"},
		{ID: "document.save", Name: "Save"},
	})
	var outcomes []Action
	tester := ui.NewTester(func(c *ui.Context) {
		host.Render(c, func(outcome Action) { outcomes = append(outcomes, outcome) })
	}, 800, 600)
	tester.Frame()
	tester.Type("save")
	tester.Frame()
	if len(outcomes) != 0 {
		t.Fatalf("query rendering routed an action to root: %+v", outcomes)
	}
	if err := tester.Click("Save"); err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || outcomes[0].Kind != Execute || outcomes[0].ID != "document.save" {
		t.Fatalf("rendered selections = %+v, want typed execute ID", outcomes)
	}
	if host.IsOpen() {
		t.Fatal("rendered execute did not close the palette")
	}

	host.Open([]Command{{ID: "document.new", Name: "New Document"}})
	tester.Key(0, ui.KeyEscape)
	tester.Frame()
	if len(outcomes) != 2 || outcomes[1].Kind != Dismiss || host.IsOpen() {
		t.Fatalf("rendered outcomes = %+v, open=%t", outcomes, host.IsOpen())
	}
}

func actionIsOnlyID(action Action) bool {
	return action.Query == "" && action.Index == 0
}
