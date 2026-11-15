package naviconpicker

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestRestoreDefaultDispatchesTypedAction(t *testing.T) {
	var actions []Action
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, ViewModel{}, func(action Action) { actions = append(actions, action) })
	}, 400, 500)
	tester.Frame()
	if err := tester.Click(locales.CommonRestoreDefaultIcon()); err != nil {
		t.Fatal(err)
	}
	for _, action := range actions {
		if action.Kind == RestoreDefault {
			return
		}
	}
	t.Fatalf("actions = %+v, want restore-default action", actions)
}

func TestQueryChooseAndDismissUseTypedActions(t *testing.T) {
	var actions []Action
	model := ViewModel{Path: "/vault/notes/Home.md", Results: []Option{{ID: "bookmark", Label: "Bookmark"}}}
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, model, func(action Action) { actions = append(actions, action) })
	}, 420, 500)
	tester.Frame()
	tester.Type("star")
	tester.Frame()
	if got := actions[len(actions)-1]; got.Kind != QueryChanged || got.Query != "star" {
		t.Fatalf("query action = %+v, want query-changed", got)
	}
	model.Query = "star"
	tester.Frame()
	if err := tester.Click("Bookmark"); err != nil {
		t.Fatal(err)
	}
	if got := actions[len(actions)-1]; got.Kind != Choose || got.ID != "bookmark" {
		t.Fatalf("choose action = %+v, want selected icon", got)
	}
	if err := tester.Click(locales.CommonCancel()); err != nil {
		t.Fatal(err)
	}
	if got := actions[len(actions)-1]; got.Kind != Close {
		t.Fatalf("dismiss action = %+v, want close", got)
	}
}
