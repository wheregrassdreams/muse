package properties

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestRenderShowsFieldsTagsAndReadOnlySystemValues(t *testing.T) {
	model := ViewModel{
		Fields: []Field{
			{Key: "tags", Label: "Tags", Kind: List, Tags: []string{"design", "notes"}, Value: "design, notes", Editable: true},
			{Key: "status", Label: "Status", Value: "draft", DisplayValue: "Draft", Editable: true},
		},
		SystemFields: []Field{{Key: "path", Label: "Path", Value: "/vault/Plan.md", System: true}},
		Backlinks:    []Backlink{{Path: "Index.md", Label: "Index", Count: 2}},
	}
	state := state{hiddenOpen: true}
	tester := ui.NewTester(func(c *ui.Context) { Render(c, &state, model, nil) }, 360, 460)
	tester.Frame()
	for _, text := range []string{"Tags", "design", "notes", "Status", "Draft", "/vault/Plan.md", "Index"} {
		if !tester.HasText(text) {
			t.Errorf("missing projected text %q: %q", text, tester.Texts())
		}
	}
}

func TestEditableValueStartsEditAndSaveEmitsSourceValue(t *testing.T) {
	var actions []Action
	state := &state{}
	model := ViewModel{Fields: []Field{{Key: "status", Label: "Status", Value: "draft", Editable: true, Kind: Text}}}
	tester := ui.NewTester(func(c *ui.Context) { Render(c, state, model, func(a Action) { actions = append(actions, a) }) }, 360, 220)
	tester.Frame()
	if err := tester.Click("Status draft"); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != BeginEdit || actions[0].FieldKey != "status" {
		t.Fatalf("begin-edit actions = %+v", actions)
	}
	actions = nil
	state.BeginEdit("status", "sidebar", "in progress")
	tester.Frame()
	if err := tester.Click(locales.CommonSave()); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != SaveEdit || actions[0].FieldKey != "status" || actions[0].Value != "in progress" {
		t.Fatalf("save actions = %+v", actions)
	}
}

func TestDeleteFieldEmitsTypedAction(t *testing.T) {
	var actions []Action
	state := &state{}
	model := ViewModel{Fields: []Field{{Key: "custom", Label: "Custom", Value: "value", Removable: true}}}
	tester := ui.NewTester(func(c *ui.Context) { Render(c, state, model, func(a Action) { actions = append(actions, a) }) }, 360, 220)
	tester.Frame()
	if err := tester.Click(locales.PropertyDelete()); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != DeleteField || actions[0].FieldKey != "custom" {
		t.Fatalf("delete actions = %+v", actions)
	}
}

func TestAddFieldEmitsCallerDraftAndKind(t *testing.T) {
	var actions []Action
	state := &state{addOpen: true, addDraft: AddDraft{Key: "priority", Value: "2", Kind: Number}}
	model := ViewModel{}
	tester := ui.NewTester(func(c *ui.Context) { Render(c, state, model, func(a Action) { actions = append(actions, a) }) }, 360, 260)
	tester.Frame()
	if err := tester.Click(locales.CommonAdd()); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != AddField || actions[0].AddDraft != model.AddDraft {
		t.Fatalf("add actions = %+v", actions)
	}
}

func TestBooleanAddDraftReportsKeyChanges(t *testing.T) {
	var actions []Action
	state := &state{addOpen: true, addDraft: AddDraft{Kind: Boolean}}
	model := ViewModel{}
	tester := ui.NewTester(func(c *ui.Context) { Render(c, state, model, func(a Action) { actions = append(actions, a) }) }, 360, 260)
	tester.Frame()
	if err := tester.Click(locales.PropertyName()); err != nil {
		t.Fatal(err)
	}
	tester.Type("flag")
	tester.Frame()
	for _, action := range actions {
		if action.Kind == AddDraftChanged && action.AddDraft.Key == "flag" && action.AddDraft.Kind == Boolean {
			return
		}
	}
	t.Fatalf("actions = %+v, want Boolean AddDraftChanged preserving key", actions)
}

func TestEmptyPropertiesStateUsesCallerMessage(t *testing.T) {
	model := ViewModel{EmptyMessage: "No metadata yet"}
	state := &state{}
	tester := ui.NewTester(func(c *ui.Context) { Render(c, state, model, nil) }, 320, 180)
	tester.Frame()
	if !tester.HasText("No metadata yet") {
		t.Fatalf("empty message missing: %q", tester.Texts())
	}
}

func TestReadOnlySystemFieldDoesNotOfferDeleteAction(t *testing.T) {
	var actions []Action
	state := &state{}
	model := ViewModel{SystemFields: []Field{{Key: "path", Label: "Path", Value: "/vault/Plan.md", System: true, Removable: true}}}
	tester := ui.NewTester(func(c *ui.Context) { Render(c, state, model, func(a Action) { actions = append(actions, a) }) }, 360, 220)
	tester.Frame()
	if tester.HasText(locales.PropertyDelete()) {
		t.Fatalf("system field rendered a delete action: %q", tester.Texts())
	}
	if len(actions) != 0 {
		t.Fatalf("unexpected actions: %+v", actions)
	}
}

func TestSummaryBacklinkAndMoreButtonsDispatchNavigationActions(t *testing.T) {
	var actions []Action
	state := &state{}
	model := ViewModel{
		DocumentType:  "Project",
		BacklinkCount: 2,
		MoreCount:     3,
		Summary: []Field{
			{Key: "status", Label: "Status", DisplayValue: "Active", Editable: true},
			{Key: "date", Label: "Date", DisplayValue: "Today", Editable: true},
			{Key: "tags", Label: "Tags", DisplayValue: "alpha", Editable: true},
			{Key: "aliases", Label: "Aliases", DisplayValue: "Main", Editable: true},
			{Key: "owner", Label: "Owner", DisplayValue: "Ari", Editable: true},
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		RenderSummary(c, state, model, func(action Action) { actions = append(actions, action) })
	}, 760, 80)
	tester.Frame()
	if err := tester.Click(locales.PropertyViewBacklinks()); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != OpenBacklinks {
		t.Fatalf("backlink action = %+v", actions)
	}
	actions = nil
	if err := tester.Click(locales.PropertyMoreCount(3)); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != OpenPanel {
		t.Fatalf("more action = %+v", actions)
	}
}

func TestSummaryPropertyDispatchesEditAction(t *testing.T) {
	var actions []Action
	state := &state{}
	tester := ui.NewTester(func(c *ui.Context) {
		RenderSummary(c, state, ViewModel{Summary: []Field{{Key: "status", DisplayValue: "Draft", Editable: true}}}, func(action Action) {
			actions = append(actions, action)
		})
	}, 420, 80)
	tester.Frame()
	if err := tester.Click("Draft"); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != BeginEdit || actions[0].FieldKey != "status" {
		t.Fatalf("summary action = %+v", actions)
	}
}
