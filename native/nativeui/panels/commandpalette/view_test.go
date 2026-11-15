package commandpalette

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestQueryNavigationExecutionAndDismissAreTypedActions(t *testing.T) {
	model := ViewModel{Commands: []Command{
		{ID: "open", Name: "Open File"},
		{ID: "save", Name: "Save Note", Hint: "⌘S"},
	}}
	var actions []Action
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, model, func(action Action) { actions = append(actions, action) })
	}, 800, 600)
	tester.Frame()
	tester.Type("draft")
	tester.Frame()
	if got := actions[len(actions)-1]; got.Kind != QueryChanged || got.Query != "draft" {
		t.Fatalf("query action = %+v, want query-changed", got)
	}
	model.Query = "draft"
	tester.Frame()
	tester.Key(0, ui.KeyDown)
	tester.Frame()
	if got := actions[len(actions)-1]; got.Kind != MoveSelection || got.Index != 1 {
		t.Fatalf("move action = %+v, want selection index 1", got)
	}
	if err := tester.Click("Save Note"); err != nil {
		t.Fatal(err)
	}
	if got := actions[len(actions)-1]; got.Kind != Execute || got.ID != "save" {
		t.Fatalf("execute action = %+v, want save command", got)
	}
	tester.Key(0, ui.KeyEscape)
	tester.Frame()
	if got := actions[len(actions)-1]; got.Kind != Dismiss {
		t.Fatalf("dismiss action = %+v, want Escape dismissal", got)
	}
}

func TestEmptyCommandListShowsNoMatchAndSubmitDismisses(t *testing.T) {
	var actions []Action
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, ViewModel{}, func(action Action) { actions = append(actions, action) })
	}, 800, 600)
	tester.Frame()
	if !tester.HasText(locales.CommandPaletteNoMatch()) {
		t.Fatalf("empty state missing: %q", tester.Texts())
	}
	tester.TypeKey(0, ui.KeyEnter, "")
	tester.Frame()
	if len(actions) > 0 && actions[len(actions)-1].Kind != Dismiss {
		t.Fatalf("empty submit action = %+v", actions[len(actions)-1])
	}
}
