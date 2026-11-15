package search

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestRenderRecentClickEmitsOpenRecent(t *testing.T) {
	open := true
	var actions []Action
	model := ViewModel{
		Open:   &open,
		Recent: []RecentItem{{Path: "notes/recent.md", Title: "Recent note", PathLabel: "notes/recent.md"}},
	}
	tester := ui.NewTester(func(c *ui.Context) { Render(c, model, func(action Action) { actions = append(actions, action) }) }, 900, 700)
	tester.Frame()
	if !tester.HasText("Recent note") || !tester.HasText(locales.WorkspaceRecentNotes()) {
		t.Fatalf("recent view missing content: %q", tester.Texts())
	}
	if err := tester.Click("notes/recent.md"); err != nil {
		t.Fatal(err)
	}
	if len(actions) == 0 || actions[len(actions)-1].Kind != OpenRecent || actions[len(actions)-1].Recent.Path != "notes/recent.md" {
		t.Fatalf("actions = %+v, want open-recent action", actions)
	}
}

func TestRenderQueryModeAndResultActions(t *testing.T) {
	open := true
	var actions []Action
	model := ViewModel{
		Open: &open, Query: "needle", Mode: FullText, Availability: Ready, CanChangeMode: true,
		Results: []Result{{Path: "notes/target.md", Title: "Target note", PathLabel: "notes/target.md", Snippet: "needle in body", Offset: 42, Revision: "r1"}},
	}
	tester := ui.NewTester(func(c *ui.Context) { Render(c, model, func(action Action) { actions = append(actions, action) }) }, 900, 700)
	tester.Frame()
	if !tester.HasText("Target note") || !tester.HasText("needle in body") {
		t.Fatalf("result view missing: %q", tester.Texts())
	}
	if err := tester.Click(locales.WorkspaceSearchLiteral()); err != nil {
		t.Fatal(err)
	}
	if got := lastAction(t, actions); got.Kind != ModeChanged || got.Mode != Literal {
		t.Fatalf("mode action = %+v", got)
	}
	if err := tester.Click("notes/target.md"); err != nil {
		t.Fatal(err)
	}
	if got := lastAction(t, actions); got.Kind != OpenResult || got.Result.Path != "notes/target.md" || got.Result.Offset != 42 || got.Result.Revision != "r1" {
		t.Fatalf("result action = %+v", got)
	}
}

func TestRenderQueryInputAndDismissAreTypedActions(t *testing.T) {
	open := true
	var actions []Action
	model := ViewModel{Open: &open, Availability: VaultRequired}
	tester := ui.NewTester(func(c *ui.Context) { Render(c, model, func(action Action) { actions = append(actions, action) }) }, 900, 700)
	tester.Frame()
	tester.Type("draft")
	tester.Frame()
	if got := lastAction(t, actions); got.Kind != QueryChanged || got.Query != "draft" {
		t.Fatalf("query action = %+v", got)
	}
	tester.Key(0, ui.KeyEscape)
	tester.Frame()
	if got := lastAction(t, actions); got.Kind != Dismiss {
		t.Fatalf("dismiss action = %+v", got)
	}
	if open {
		t.Fatal("dialog did not close on Escape")
	}
}

func TestRenderAvailabilityAndModePresentation(t *testing.T) {
	open := true
	model := ViewModel{Open: &open, Query: "query", Availability: IndexBuilding}
	tester := ui.NewTester(func(c *ui.Context) { Render(c, model, nil) }, 900, 700)
	tester.Frame()
	if !tester.HasText(locales.VaultIndexBuilding()) {
		t.Fatalf("index building state missing: %q", tester.Texts())
	}
	if !tester.HasText(locales.WorkspaceQuickKeys()) {
		t.Fatalf("keyboard help missing: %q", tester.Texts())
	}
}

func lastAction(t *testing.T, actions []Action) Action {
	t.Helper()
	if len(actions) == 0 {
		t.Fatal("no action was dispatched")
	}
	return actions[len(actions)-1]
}
