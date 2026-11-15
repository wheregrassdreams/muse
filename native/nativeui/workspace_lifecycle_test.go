package nativeui

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"muse/events"
	"muse/workspace"
)

type inMemoryWorkspace struct {
	state workspace.State
}

func (r *inMemoryWorkspace) Load() (workspace.State, error) { return r.state, nil }
func (r *inMemoryWorkspace) Save(state workspace.State) error {
	r.state = state
	return nil
}

func TestWorkspaceLifecyclePersistsAndRestoresActiveDocumentAndCaret(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	root := t.TempDir()
	path := filepath.Join(root, "active.md")
	source := "first line\nsecond line\nthird line\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	store := &inMemoryWorkspace{}
	workspaceEvents := events.New[workspace.WorkspaceRestored]()
	first := New()
	first.SetDependencies(Dependencies{Workspace: store, WorkspaceEvents: workspaceEvents})
	first.setWorkspace(root)
	first.openPathNow(path)
	caret := len("first line\nsecond")
	first.ed.Select(caret, caret)
	first.persistWorkspaceState()
	want := workspace.State{
		Version: 1, VaultPath: root, ActiveTab: "main",
		Tabs: []workspace.Tab{{ID: "main", ActivePaneID: "main", Layout: workspace.Leaf(workspace.Pane{
			ID: "main", Document: workspace.Document{Path: path, CursorOffset: caret},
		})}},
	}
	if got := store.state; !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted workspace = %+v, want %+v", got, want)
	}

	// A new App instance models a process restart. Restore publishes the
	// workspace event, whose consumer opens the note and restores its caret.
	second := New()
	second.SetDependencies(Dependencies{Workspace: store, WorkspaceEvents: workspaceEvents})
	second.restoreWorkspaceState()
	if second.root != root || second.path != path {
		t.Fatalf("restored root/path = %q / %q, want %q / %q", second.root, second.path, root, path)
	}
	if got := second.ed.Source(); got != source {
		t.Fatalf("restored source = %q, want %q", got, source)
	}
	if got := second.ed.SelectionRange().End; got != caret {
		t.Fatalf("restored caret = %d, want %d", got, caret)
	}
}

func TestWorkspaceLifecycleUnsubscribesOnClose(t *testing.T) {
	store := &inMemoryWorkspace{}
	dispatcher := events.New[workspace.WorkspaceRestored]()
	app := New()
	app.SetDependencies(Dependencies{Workspace: store, WorkspaceEvents: dispatcher})
	app.closeLifecycle()

	root := t.TempDir()
	dispatcher.Publish(workspace.WorkspaceRestored{State: workspace.State{Version: 1, VaultPath: root}})
	if app.root != "" {
		t.Fatalf("closed App still handled workspace event, root = %q", app.root)
	}
}

func TestMissingWorkspaceRepositoryMakesRestoreAndPersistSafeNoOps(t *testing.T) {
	app := New()
	app.SetWorkspaceRepository(nil)
	app.restoreWorkspaceState()
	app.persistWorkspaceState()
	if app.workspaceRestorer != nil {
		t.Fatal("nil repository created a workspace restorer")
	}
}
