package commandpalette

import "testing"

func TestHandleOwnsQuerySelectionAndOpenLifecycle(t *testing.T) {
	handle := NewHandle()
	if handle.IsOpen() {
		t.Fatal("new palette handle is open")
	}

	commands := []Command{
		{ID: "document.new", Name: "New Document"},
		{ID: "document.save", Name: "Save"},
		{ID: "workspace.search", Name: "Search Workspace"},
	}
	handle.Open()
	if !handle.IsOpen() {
		t.Fatal("opening palette did not update feature state")
	}
	handle.Apply(Action{Kind: MoveSelection, Index: 2})
	if got := handle.ViewModel(commands).Selected; got != 2 {
		t.Fatalf("selected command = %d, want 2", got)
	}
	handle.Apply(Action{Kind: QueryChanged, Query: "save"})
	model := handle.ViewModel(commands)
	if model.Query != "save" || len(model.Commands) != 1 || model.Commands[0].ID != "document.save" || model.Selected != 0 {
		t.Fatalf("filtered model = %+v, want one reset selection for Save", model)
	}

	handle.Close()
	if handle.IsOpen() {
		t.Fatal("closing palette did not update feature state")
	}
	handle.Open()
	model = handle.ViewModel(commands)
	if model.Query != "" || model.Selected != 0 || len(model.Commands) != len(commands) {
		t.Fatalf("reopened model = %+v, want initial query and selection", model)
	}
}
