package filetree

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

type testPayload struct{ IDs []string }

var testDragPort = DragPort[testPayload]{
	Make: func(_ string, ids []string) testPayload { return testPayload{IDs: cloneIDs(ids)} },
	IDs:  func(payload testPayload) []string { return payload.IDs },
}

func render(c *ui.Context, state *state, model ViewModel, dispatch func(Action[testPayload])) RenderResult {
	return Render(c, &Handle{state: state}, model, testDragPort, dispatch)
}

func TestClickSelectsProjectedEntry(t *testing.T) {
	var actions []Action[testPayload]
	state := newState()
	model := ViewModel{
		Entries: []Entry{
			{ID: "folder", Label: "Projects", Kind: Directory, CanSelect: true, CanExpand: true},
			{ID: "note", Label: "Roadmap.md", Kind: File, Depth: 1, CanSelect: true, CanOpen: true},
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		render(c, state, model, func(a Action[testPayload]) { actions = append(actions, a) })
	}, 360, 180)
	tester.Frame()
	tester.ClickAt(180, 43)
	tester.Frame()
	if len(actions) != 2 || actions[0].Kind != SelectEntry || actions[0].ID != "note" || actions[1].Kind != OpenEntry || actions[1].ID != "note" {
		t.Fatalf("actions = %+v, want select then open note", actions)
	}
}

func TestExpansionControlEmitsToggleAction(t *testing.T) {
	var actions []Action[testPayload]
	state := newState()
	model := ViewModel{Entries: []Entry{{ID: "folder", Label: "Projects", Kind: Directory, CanSelect: true, CanExpand: true}}}
	tester := ui.NewTester(func(c *ui.Context) {
		render(c, state, model, func(a Action[testPayload]) { actions = append(actions, a) })
	}, 320, 120)
	tester.Frame()
	tester.ClickAt(13, 14)
	tester.Frame()
	if len(actions) != 1 || actions[0].Kind != ToggleEntry || actions[0].ID != "folder" {
		t.Fatalf("actions = %+v, want toggle folder", actions)
	}
}

func TestSubmittedFileEmitsOpenAction(t *testing.T) {
	var actions []Action[testPayload]
	state := newState()
	state.SetFocused(true)
	model := ViewModel{Entries: []Entry{{ID: "note", Label: "Roadmap.md", Kind: File, CanSelect: true, CanOpen: true}}}
	tester := ui.NewTester(func(c *ui.Context) {
		render(c, state, model, func(a Action[testPayload]) { actions = append(actions, a) })
	}, 320, 120)
	tester.Frame()
	tester.ClickAt(160, 14)
	tester.Frame()
	actions = nil
	tester.Key(0, ui.KeyEnter)
	tester.Frame()
	if len(actions) != 1 || actions[0].Kind != OpenEntry || actions[0].ID != "note" {
		t.Fatalf("actions = %+v, want open note", actions)
	}
}

func TestKeyboardMoveEmitsSelectionForFocusedTree(t *testing.T) {
	var actions []Action[testPayload]
	state := newState()
	state.SetFocused(true)
	state.SetSelection([]string{"one"})
	model := ViewModel{
		Entries: []Entry{
			{ID: "one", Label: "One.md", Kind: File, CanSelect: true},
			{ID: "two", Label: "Two.md", Kind: File, CanSelect: true},
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		render(c, state, model, func(a Action[testPayload]) { actions = append(actions, a) })
	}, 320, 120)
	tester.Frame()
	tester.ClickAt(160, 14)
	tester.Frame()
	actions = nil
	tester.Key(0, ui.KeyDown)
	tester.Frame()
	if len(actions) != 1 || actions[0].Kind != SelectEntry || actions[0].ID != "two" {
		t.Fatalf("keyboard actions = %+v, want selection of next row", actions)
	}
}

func TestToolbarCommandAndDropAction(t *testing.T) {
	var actions []Action[testPayload]
	state := newState()
	model := ViewModel{
		Commands: []Command{{ID: "new-note", Label: "New note", Enabled: true}},
		Entries: []Entry{
			{ID: "folder", Label: "Projects", Kind: Directory, CanSelect: true, CanDrop: true},
			{ID: "note", Label: "Roadmap.md", Kind: File, CanSelect: true, CanDrag: true, DragIDs: []string{"note"}},
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		render(c, state, model, func(a Action[testPayload]) { actions = append(actions, a) })
	}, 360, 180)
	tester.Frame()
	if err := tester.Click("New note"); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != InvokeCommand || actions[0].CommandID != "new-note" {
		t.Fatalf("command actions = %+v", actions)
	}
}

func TestRowContextMenuReturnsOpaqueCommand(t *testing.T) {
	var actions []Action[testPayload]
	state := newState()
	model := ViewModel{Entries: []Entry{{ID: "note", Label: "Roadmap.md", Kind: File, ContextActions: []Command{{ID: "trash", Label: "Move to Trash", Enabled: true}}}}}
	tester := ui.NewTester(func(c *ui.Context) {
		render(c, state, model, func(a Action[testPayload]) { actions = append(actions, a) })
	}, 320, 120)
	tester.Frame()
	tester.RightClickAt(160, 14)
	if err := tester.ChooseMenuItem("Move to Trash"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if len(actions) != 1 || actions[0].Kind != InvokeContextCommand || actions[0].ID != "note" || actions[0].CommandID != "trash" {
		t.Fatalf("context actions = %+v", actions)
	}
}

func TestRootContextMenuUsesEmptyTargetID(t *testing.T) {
	var actions []Action[testPayload]
	state := newState()
	model := ViewModel{EmptyMessage: "Empty", RootContextActions: []Command{{ID: "new-folder", Label: "New folder", Enabled: true}}}
	tester := ui.NewTester(func(c *ui.Context) {
		render(c, state, model, func(a Action[testPayload]) { actions = append(actions, a) })
	}, 320, 120)
	tester.Frame()
	tester.RightClickAt(160, 60)
	if err := tester.ChooseMenuItem("New folder"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if len(actions) != 1 || actions[0].Kind != InvokeContextCommand || actions[0].ID != "" || actions[0].CommandID != "new-folder" {
		t.Fatalf("root context actions = %+v", actions)
	}
}

func TestPreviewHoverActionsOnlyFireOnEntryTransitions(t *testing.T) {
	var actions []Action[testPayload]
	state := newState()
	model := ViewModel{Entries: []Entry{{ID: "image", Label: "Photo.png", Kind: File, Previewable: true}}}
	tester := ui.NewTester(func(c *ui.Context) {
		render(c, state, model, func(a Action[testPayload]) {
			actions = append(actions, a)
		})
	}, 320, 120)
	tester.Frame()
	tester.Move(140, 14)
	tester.Frame()
	tester.Frame()
	if len(actions) != 1 || actions[0].Kind != HoverEntry || actions[0].ID != "image" {
		t.Fatalf("hover actions repeated or missing: %+v", actions)
	}
	tester.Move(310, 110)
	tester.Frame()
	if len(actions) != 2 || actions[1].Kind != LeaveEntry || actions[1].ID != "image" {
		t.Fatalf("leave action missing: %+v", actions)
	}
}

func TestDragAndDropEmitsGenericAction(t *testing.T) {
	var actions []Action[testPayload]
	state := newState()
	model := ViewModel{Entries: []Entry{
		{ID: "folder", Label: "Projects", Kind: Directory, CanSelect: true, CanDrop: true},
		{ID: "note", Label: "Roadmap.md", Kind: File, CanSelect: true, CanDrag: true, DragIDs: []string{"note", "other"}},
		{ID: "other", Label: "Other.md", Kind: File, CanSelect: true},
	}}
	tester := ui.NewTester(func(c *ui.Context) {
		render(c, state, model, func(a Action[testPayload]) {
			actions = append(actions, a)
			if a.Kind == StartDrag && a.ID == "note" {
				model.Entries[1].DragActive = true
			}
		})
	}, 320, 120)
	tester.Frame()
	tester.Press(160, 43)
	tester.Move(160, 14)
	tester.Frame()
	started := false
	for _, action := range actions {
		if action.Kind == StartDrag && action.ID == "note" && len(action.ItemIDs) == 2 && action.ItemIDs[0] == "note" && action.ItemIDs[1] == "other" && action.Payload.IDs[0] == "note" {
			started = true
		}
	}
	if !started {
		t.Fatalf("drag start contract action missing: %+v", actions)
	}
	tester.Frame()
	starts := 0
	for _, action := range actions {
		if action.Kind == StartDrag {
			starts++
		}
	}
	if starts != 1 {
		t.Fatalf("start drag dispatched %d times, actions=%+v", starts, actions)
	}
	tester.Release(160, 14)
	tester.Frame()
	for _, action := range actions {
		if action.Kind == DropEntries && action.ID == "folder" && len(action.ItemIDs) == 2 && action.ItemIDs[0] == "note" && action.ItemIDs[1] == "other" && action.Payload.IDs[0] == "note" {
			return
		}
	}
	t.Fatalf("actions = %+v, want opaque drop request onto folder", actions)
}

func TestSingleDragUsesEntryAsPayloadAndDropsOnTargetRow(t *testing.T) {
	var actions []Action[testPayload]
	state := newState()
	var madeEntry string
	var madeIDs []string
	drag := DragPort[testPayload]{
		Make: func(entryID string, ids []string) testPayload {
			madeEntry = entryID
			madeIDs = cloneIDs(ids)
			return testPayload{IDs: cloneIDs(ids)}
		},
		IDs: func(payload testPayload) []string { return payload.IDs },
	}
	model := ViewModel{Entries: []Entry{
		{ID: "folder", Label: "Projects", Kind: Directory, CanSelect: true, CanDrop: true},
		{ID: "note", Label: "Roadmap.md", Kind: File, CanSelect: true, CanDrag: true},
	}}
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, &Handle{state: state}, model, drag, func(a Action[testPayload]) { actions = append(actions, a) })
	}, 320, 120)
	tester.Frame()
	tester.Press(160, 43)
	tester.Move(160, 14)
	tester.Frame()
	tester.Release(160, 14)
	tester.Frame()
	if madeEntry != "note" || len(madeIDs) != 1 || madeIDs[0] != "note" {
		t.Fatalf("single-item drag projection = entry %q ids %v, want note", madeEntry, madeIDs)
	}
	for _, action := range actions {
		if action.Kind == DropEntries && action.ID == "folder" && len(action.ItemIDs) == 1 && action.ItemIDs[0] == "note" && action.Payload.IDs[0] == "note" {
			return
		}
	}
	t.Fatalf("actions = %+v, want a typed single-item drop on folder", actions)
}

func TestInjectedScrollStateReceivesWheelInput(t *testing.T) {
	entries := make([]Entry, 30)
	for i := range entries {
		entries[i] = Entry{ID: fmt.Sprintf("note-%02d", i), Label: fmt.Sprintf("Note %02d", i), Kind: File, CanSelect: true}
	}
	state := newState()
	model := ViewModel{Entries: entries}
	tester := ui.NewTester(func(c *ui.Context) { render(c, state, model, nil) }, 300, 120)
	tester.Frame()
	tester.Scroll(150, 90, 0, 100)
	tester.Frame()
	if state.ScrollOffset().Y <= 0 {
		t.Fatalf("scroll state not updated: %+v", state.ScrollOffset())
	}
}

func TestCloneIDsDetachesDropActionPayload(t *testing.T) {
	input := []string{"a", "b"}
	copy := cloneIDs(input)
	input[0] = "changed"
	if copy[0] != "a" {
		t.Fatal("drag projection copy aliases caller memory")
	}
}
