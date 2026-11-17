package agent

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestApprovalDispatchesToolDecision(t *testing.T) {
	var actions []Action
	state := newState()
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, state, View{Messages: []Message{{Role: Assistant, Parts: []Part{{
			Kind: ToolPart,
			Tool: Tool{ID: "call-17", Name: "Edit note", Target: "notes/today.md", Status: ToolNeedsApproval, Summary: "Replace the selected paragraph."},
		}}}}}, func(action Action) { actions = append(actions, action) })
	}, 420, 600)
	tester.Frame()
	if err := tester.Click(locales.AiApproveChange()); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != ApproveTool || actions[0].ToolID != "call-17" {
		t.Fatalf("actions = %+v, want approve-tool for call-17", actions)
	}
}

func TestComposerDispatchesSendWithDraft(t *testing.T) {
	var actions []Action
	state := newState()
	state.SetDraft("Summarize this note")
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, state, View{}, func(action Action) { actions = append(actions, action) })
	}, 420, 500)
	tester.Frame()
	if err := tester.Click(locales.CommonSend()); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != Send || actions[0].Text != "Summarize this note" {
		t.Fatalf("actions = %+v, want send with the supplied draft", actions)
	}
	if got := state.Draft(); got != "" {
		t.Fatalf("draft after send = %q, want empty", got)
	}
}

func TestExpandedPartDispatchesItsLocation(t *testing.T) {
	var actions []Action
	model := View{Messages: []Message{{Role: Assistant, Parts: []Part{
		{Kind: TextPart, Text: "Result"},
		{Kind: ThoughtPart, Text: "I checked the note.", Expanded: true},
	}}}}
	state := newState()
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, state, model, func(action Action) { actions = append(actions, action) })
	}, 420, 500)
	tester.Frame()
	if err := tester.Click(locales.AiThinkProcess()); err != nil {
		t.Fatal(err)
	}
	if len(actions) == 0 || actions[len(actions)-1].Kind != TogglePart || actions[len(actions)-1].MessageIndex != 0 || actions[len(actions)-1].PartIndex != 1 {
		t.Fatalf("actions = %+v, want toggle for message 0 part 1", actions)
	}
}

func TestSetupPromptDispatchesOpenSettings(t *testing.T) {
	var actions []Action
	state := newState()
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, state, View{SetupTitle: "Configure assistant", SetupButton: "Open settings"}, func(action Action) {
			actions = append(actions, action)
		})
	}, 420, 500)
	tester.Frame()
	if err := tester.Click("Open settings"); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != OpenSettings {
		t.Fatalf("actions = %+v, want open-settings", actions)
	}
}

func TestEmptyConversationSuggestionDispatchesText(t *testing.T) {
	var actions []Action
	state := newState()
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, state, View{Suggestions: []string{"Summarize this note"}}, func(action Action) {
			actions = append(actions, action)
		})
	}, 420, 500)
	tester.Frame()
	if err := tester.Click("Summarize this note"); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != UseSuggestion || actions[0].Text != "Summarize this note" {
		t.Fatalf("actions = %+v, want supplied suggestion", actions)
	}
	if got := state.Draft(); got != "Summarize this note" {
		t.Fatalf("draft after suggestion = %q, want supplied suggestion", got)
	}
}
