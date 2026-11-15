package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"muse/events"
	"muse/platform/filesystem/workspacestore"
	"muse/workspace"
)

func TestRestorePublishesValidatedWorkspaceState(t *testing.T) {
	dispatcher := events.New[workspace.WorkspaceRestored]()
	store := workspacestore.New(filepath.Join(t.TempDir(), "workspace.json"))
	root := t.TempDir()
	path := filepath.Join(root, "Note.md")
	if err := os.WriteFile(path, []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}
	const tabID, paneID = "main", "main"
	want := workspace.State{
		Version: 1, VaultPath: root, ActiveTab: tabID,
		Tabs: []workspace.Tab{{
			ID: tabID, ActivePaneID: paneID,
			Layout: workspace.Leaf(workspace.Pane{ID: paneID, Document: workspace.Document{Path: path, CursorOffset: 4}}),
		}},
	}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	var received workspace.State
	dispatcher.Subscribe(func(event workspace.WorkspaceRestored) { received = event.State })
	state, err := NewRestorer(store, dispatcher).Restore()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(state, want) || !reflect.DeepEqual(received, want) {
		t.Fatalf("restore = %+v event = %+v, want %+v", state, received, want)
	}
}

func TestRestorerWithoutRepositoryReturnsUnavailableInsteadOfPanicking(t *testing.T) {
	var restorer *Restorer
	if _, err := restorer.Restore(); err != ErrUnavailable {
		t.Fatalf("nil Restorer.Restore error = %v, want %v", err, ErrUnavailable)
	}
	if err := restorer.Persist(workspace.State{}); err != ErrUnavailable {
		t.Fatalf("nil Restorer.Persist error = %v, want %v", err, ErrUnavailable)
	}
	if _, err := NewRestorer(nil, nil).Restore(); err != ErrUnavailable {
		t.Fatalf("unconfigured Restorer.Restore error = %v, want %v", err, ErrUnavailable)
	}
	if err := NewRestorer(nil, nil).Persist(workspace.State{}); err != ErrUnavailable {
		t.Fatalf("unconfigured Restorer.Persist error = %v, want %v", err, ErrUnavailable)
	}
}
