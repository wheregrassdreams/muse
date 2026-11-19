package filetree

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
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

func TestPlainClickSelectsEntryWithoutStartingDrag(t *testing.T) {
	state := newState()
	model := ViewModel{Entries: []Entry{{ID: "note.md", Label: "note.md", Kind: File, CanSelect: true, CanOpen: true, CanDrag: true}}}
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, &Handle{state: state}, model, testDragPort, nil)
	}, 360, 160)
	tester.Frame()
	before, ok := tester.Find("note.md")
	if !ok {
		t.Fatal("tree row missing")
	}
	tester.ClickAt(before.X+4, before.Y+10)
	tester.Frame()
	after, ok := tester.Find("note.md")
	if !ok || before.H != after.H || before.Y != after.Y {
		t.Fatalf("row geometry changed on click: before=%+v after=%+v", before, after)
	}
	if state.DragPreview().Active || !state.IsSelected("note.md") {
		t.Fatalf("plain click changed drag state or missed selection: drag=%+v selected=%v text=%v", state.DragPreview(), state.SelectedIDs(), tester.Texts())
	}
}

func TestModifierSelectionDoesNotOpenOrToggleTreeRows(t *testing.T) {
	state := newState()
	var actions []Action[testPayload]
	model := ViewModel{Entries: []Entry{
		{ID: "a.md", Label: "a.md", Kind: File, CanSelect: true, CanOpen: true},
		{ID: "b.md", Label: "b.md", Kind: File, CanSelect: true, CanOpen: true},
		{ID: "c.md", Label: "c.md", Kind: File, CanSelect: true, CanOpen: true},
		{ID: "Folder", Label: "Folder", Kind: Directory, CanSelect: true, CanExpand: true},
	}}
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, &Handle{state: state}, model, testDragPort, func(action Action[testPayload]) { actions = append(actions, action) })
	}, 360, 220)
	tester.Frame()
	clickProjectedEntry(t, tester, 0, "a.md")
	tester.Frame()
	actions = nil
	clickProjectedEntry(t, tester, ui.Cmd, "b.md")
	tester.Frame()
	if !state.IsSelected("a.md") || !state.IsSelected("b.md") || containsAction(actions, OpenEntry) {
		t.Fatalf("modifier click selection/actions = %v/%+v", state.SelectedIDs(), actions)
	}
	clickProjectedEntry(t, tester, ui.Cmd, "b.md")
	tester.Frame()
	if state.IsSelected("b.md") || !state.IsSelected("a.md") {
		t.Fatalf("Cmd click did not toggle selection: %v", state.SelectedIDs())
	}
	clickProjectedEntry(t, tester, ui.Cmd, "b.md")
	tester.Frame()
	clickProjectedEntry(t, tester, ui.Shift, "c.md")
	tester.Frame()
	if len(state.SelectedIDs()) != 3 || containsAction(actions, OpenEntry) {
		t.Fatalf("Shift range selection/actions = %v/%+v", state.SelectedIDs(), actions)
	}
	clickProjectedEntry(t, tester, ui.Cmd, "Folder")
	tester.Frame()
	if state.IsExpanded("Folder") || !state.IsSelected("Folder") {
		t.Fatalf("modifier click toggled folder or missed selection: expanded=%v selection=%v", state.IsExpanded("Folder"), state.SelectedIDs())
	}
}

func clickProjectedEntry(t *testing.T, tester *ui.Tester, mods ui.Modifiers, label string) {
	t.Helper()
	entry, ok := tester.Find(label)
	if !ok {
		t.Fatalf("entry %q missing; texts=%v", label, tester.Texts())
	}
	tester.ClickAtWith(mods, entry.X+entry.W/2, entry.Y+entry.H/2)
}

func containsAction(actions []Action[testPayload], kind ActionKind) bool {
	for _, action := range actions {
		if action.Kind == kind {
			return true
		}
	}
	return false
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
	if len(actions) != 1 || actions[0].Kind != SubmitEntry || actions[0].ID != "note" {
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
	if len(actions) != 1 || actions[0].Kind != NavigateEntry || actions[0].ID != "two" {
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

func TestEmptyStateOpenMenuDispatchesRecentDestination(t *testing.T) {
	recent := RecentItem{Path: "/vault/notes.md", Label: "notes.md"}
	model := ViewModel{
		EmptyActionLabel: "Open file or folder",
		EmptyActions:     []Command{{ID: CommandOpenFile, Label: "Open file", Enabled: true}},
		Recent:           []RecentItem{recent},
	}
	var actions []Action[testPayload]
	tester := ui.NewTester(func(c *ui.Context) {
		render(c, newState(), model, func(action Action[testPayload]) { actions = append(actions, action) })
	}, 320, 120)
	tester.Frame()
	if err := tester.Click(model.EmptyActionLabel); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem(recent.Label); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != OpenRecent || actions[0].Recent != recent {
		t.Fatalf("recent action = %+v, want path-bearing OpenRecent action", actions)
	}
}

func TestDragIntoBlankPanelAreaTargetsWorkspaceRoot(t *testing.T) {
	handle := NewHandle()
	model := ModelInput{
		RootLabel:   "/vault",
		RootCanDrop: true,
		Entries: []ModelEntryInput{
			{Row: Row{Path: "/vault/Folder", Directory: true}},
			{Row: Row{Path: "/vault/note.md"}},
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		RenderPanel(c, handle, PanelInput[testPayload]{Model: model, Drag: testDragPort})
	}, 400, 360)
	tester.Frame()
	dragHandle, ok := tester.Find(locales.TreeDragHandleLabel("note.md"))
	if !ok {
		t.Fatal("note drag handle missing")
	}
	tester.Press(dragHandle.X+4, dragHandle.Y+14)
	tester.Move(dragHandle.X+14, dragHandle.Y+14)
	tester.Frame()
	tester.Move(330, 300)
	tester.Frame()
	if got := handle.DragOperationDisplay(); got.Kind != DragOperationMoveToFolder || got.TargetLabel != "vault" {
		t.Fatalf("blank panel drop target = %+v, want workspace root", got)
	}
}

func TestPreviewHoverActionsOnlyFireOnEntryTransitions(t *testing.T) {
	var actions []Action[testPayload]
	state := newState()
	model := ViewModel{Entries: []Entry{{ID: "image", Label: "Photo.png", Kind: File, CanDrag: true, Previewable: true}}}
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

func TestDraggingPreviewableEntryClearsAndKeepsHoverPreviewClosed(t *testing.T) {
	var actions []Action[testPayload]
	state := newState()
	model := ViewModel{Entries: []Entry{{ID: "image", Label: "Photo.png", Kind: File, CanDrag: true, Previewable: true}}}
	tester := ui.NewTester(func(c *ui.Context) {
		render(c, state, model, func(action Action[testPayload]) { actions = append(actions, action) })
	}, 320, 120)
	tester.Frame()
	row, ok := tester.Find("Photo.png")
	if !ok {
		t.Fatal("image row missing")
	}
	tester.Move(row.X+row.W/2, row.Y+row.H/2)
	tester.Frame()
	if state.PreviewID() != "image" {
		t.Fatalf("hover preview was not established before drag: %q", state.PreviewID())
	}
	hoverCount := 0
	for _, action := range actions {
		if action.Kind == HoverEntry {
			hoverCount++
		}
	}
	handle, ok := tester.Find(locales.TreeDragHandleLabel("Photo.png"))
	if !ok {
		t.Fatal("image drag handle missing")
	}
	tester.Press(handle.X+4, handle.Y+14)
	tester.Move(handle.X+14, handle.Y+14)
	tester.Frame()
	if !state.DragPreview().Active {
		t.Fatal("drag threshold did not start")
	}
	if state.PreviewID() != "" {
		t.Fatalf("hover preview reopened during drag: %q", state.PreviewID())
	}
	currentHoverCount := 0
	for _, action := range actions {
		if action.Kind == HoverEntry {
			currentHoverCount++
		}
	}
	if currentHoverCount != hoverCount {
		t.Fatalf("hover was emitted during drag: %+v", actions)
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

func TestPanelEmptyWorkspaceReturnsTypedOpenCommand(t *testing.T) {
	var actions []Action[testPayload]
	handle := NewHandle()
	tester := ui.NewTester(func(c *ui.Context) {
		RenderPanel(c, handle, PanelInput[testPayload]{
			Model: ModelInput{
				EmptyActionLabel: "Open file or folder",
				EmptyActions: []Command{
					{ID: CommandOpenFile, Label: "Open file", Enabled: true},
					{ID: CommandOpenFolder, Label: "Open folder", Enabled: true},
				},
			},
			Dispatch: func(action Action[testPayload]) { actions = append(actions, action) },
		})
	}, 360, 180)
	tester.Frame()
	if err := tester.Click("Open file or folder"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if err := tester.ChooseMenuItem("Open folder"); err != nil {
		t.Fatalf("choose open folder: %v; menu=%v", err, tester.Menu())
	}
	tester.Frame()
	if len(actions) != 1 || actions[0].Kind != InvokeCommand || actions[0].CommandID != CommandOpenFolder {
		t.Fatalf("empty-state action = %+v, want typed open-folder command", actions)
	}
}
