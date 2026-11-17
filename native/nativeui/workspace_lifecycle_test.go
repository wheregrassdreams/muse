package nativeui

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"muse/events"
	"muse/nativeui/workbench/hosting"
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
	withDocumentPorts(t, first)
	first.setWorkspace(root)
	first.openPathNow(path)
	caret := len("first line\nsecond")
	first.noteEditor().NavigationCommands().Select(caret, caret)
	first.persistWorkspaceState()
	want := workspace.State{
		Version: 1, VaultPath: root, ActiveTab: "main",
		Tabs: []workspace.Tab{{ID: "main", ActivePaneID: "editor", Layout: workspace.Leaf(workspace.Pane{
			ID: "editor", Document: workspace.Document{Path: path, CursorOffset: caret},
		})}},
	}
	if got := store.state; !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted workspace = %+v, want %+v", got, want)
	}

	// A new App instance models a process restart. Restore publishes the
	// workspace event, whose consumer opens the note and restores its caret.
	second := New()
	second.SetDependencies(Dependencies{Workspace: store, WorkspaceEvents: workspaceEvents})
	withDocumentPorts(t, second)
	second.restoreWorkspaceState()
	if second.root != root || second.documentPath() != path {
		t.Fatalf("restored root/path = %q / %q, want %q / %q", second.root, second.documentPath(), root, path)
	}
	if second.workspacePane != "editor" {
		t.Fatalf("restored active pane = %q, want editor", second.workspacePane)
	}
	if got := second.documentSource(); got != source {
		t.Fatalf("restored source = %q, want %q", got, source)
	}
	if got := second.noteEditor().NavigationCommands().SelectionRange().End; got != caret {
		t.Fatalf("restored caret = %d, want %d", got, caret)
	}
}

func TestWorkspaceLifecycleRestoresSplitPanesAndPreservesInactiveTabs(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	root := t.TempDir()
	leftPath := filepath.Join(root, "left.md")
	rightPath := filepath.Join(root, "right.md")
	otherPath := filepath.Join(root, "other.md")
	for path, source := range map[string]string{
		leftPath:  "left note\n",
		rightPath: "right note\n",
		otherPath: "other tab\n",
	} {
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store := &inMemoryWorkspace{state: workspace.State{
		Version: 1, VaultPath: root, ActiveTab: "work",
		Tabs: []workspace.Tab{
			{
				ID: "work", ActivePaneID: "right",
				Layout: workspace.Branch(workspace.Horizontal, .42,
					workspace.Leaf(workspace.Pane{ID: "left", Document: workspace.Document{Path: leftPath, CursorOffset: 2}}),
					workspace.Leaf(workspace.Pane{ID: "right", Document: workspace.Document{Path: rightPath, CursorOffset: 5}}),
				),
			},
			{ID: "later", ActivePaneID: "other", Layout: workspace.Leaf(workspace.Pane{
				ID: "other", Document: workspace.Document{Path: otherPath},
			})},
		},
	}}
	dispatcher := events.New[workspace.WorkspaceRestored]()
	app := New()
	app.SetDependencies(Dependencies{Workspace: store, WorkspaceEvents: dispatcher})
	withDocumentPorts(t, app)
	app.restoreWorkspaceState()

	if got := app.documentPath(); got != rightPath {
		t.Fatalf("focused document = %q, want %q", got, rightPath)
	}
	if got := app.noteEditor().NavigationCommands().SelectionRange().End; got != 5 {
		t.Fatalf("focused caret = %d, want 5", got)
	}
	if len(app.workspacePaneHandles) != 2 {
		t.Fatalf("restored pane handles = %d, want 2", len(app.workspacePaneHandles))
	}
	if got := app.workspaceSession.Snapshot(); !reflect.DeepEqual(got, store.state) {
		t.Fatalf("restored session = %+v, want %+v", got, store.state)
	}

	app.noteEditor().NavigationCommands().Select(7, 7)
	app.persistWorkspaceState()
	if len(store.state.Tabs) != 2 || store.state.ActiveTab != "work" {
		t.Fatalf("persist dropped tabs or active tab: %+v", store.state)
	}
	active, ok := workspaceActiveTab(store.state)
	if !ok || active.ActivePaneID != "right" || active.Layout.Split == nil {
		t.Fatalf("persist dropped active split layout: %+v", store.state)
	}
	doc, hasDoc := store.state.ActiveDocument()
	if !hasDoc || doc.Path != rightPath || doc.CursorOffset != 7 {
		t.Fatalf("persisted active document = %+v, %v", doc, hasDoc)
	}
}

func TestSwitchingWorkspaceClosesOldPanesAndStartsWithEmptySession(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	oldRoot, newRoot := t.TempDir(), t.TempDir()
	leftPath := filepath.Join(oldRoot, "left.md")
	rightPath := filepath.Join(oldRoot, "right.md")
	newPath := filepath.Join(newRoot, "fresh.md")
	for path, source := range map[string]string{leftPath: "old left", rightPath: "old right", newPath: "new vault"} {
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app := New()
	withDocumentPorts(t, app)
	app.setWorkspace(oldRoot)
	oldState := workspace.State{
		Version: 1, VaultPath: oldRoot, ActiveTab: "work",
		Tabs: []workspace.Tab{{
			ID: "work", ActivePaneID: "right",
			Layout: workspace.Branch(workspace.Horizontal, .5,
				workspace.Leaf(workspace.Pane{ID: "left", Document: workspace.Document{Path: leftPath}}),
				workspace.Leaf(workspace.Pane{ID: "right", Document: workspace.Document{Path: rightPath}}),
			),
		}},
	}
	app.applyRestoredWorkspace(oldState)
	oldHandles := []window.Handle{app.workspacePaneHandles["left"], app.workspacePaneHandles["right"]}
	if len(app.workspacePaneHandles) != 2 || app.documentPath() != rightPath {
		t.Fatalf("old workspace was not set up: handles=%d path=%q", len(app.workspacePaneHandles), app.documentPath())
	}
	for _, handle := range oldHandles {
		if !app.windows.Contains(handle) {
			t.Fatalf("old pane handle %v was not live before switch", handle)
		}
	}

	app.setWorkspace(newRoot)
	state := app.workspaceSession.Snapshot()
	if state.VaultPath != newRoot || state.ActiveTab != "" || len(state.Tabs) != 0 {
		t.Fatalf("workspace after vault switch = %+v, want an empty state for %q", state, newRoot)
	}
	if app.documentPath() != "" || app.documentSource() != "" {
		t.Fatalf("old document remained visible after switch: path=%q source=%q", app.documentPath(), app.documentSource())
	}
	if len(app.workspacePaneHandles) != 1 || !app.windows.Contains(app.editorPaneHandle) {
		t.Fatalf("new empty editor pane not initialized: handles=%d active=%v", len(app.workspacePaneHandles), app.windows.Contains(app.editorPaneHandle))
	}
	for _, handle := range oldHandles {
		if app.windows.Contains(handle) {
			t.Fatalf("old vault pane handle %v is still live", handle)
		}
	}

	app.openPathNow(newPath)
	state = app.workspaceSession.Snapshot()
	doc, ok := state.ActiveDocument()
	if !ok || state.VaultPath != newRoot || doc.Path != newPath {
		t.Fatalf("new document did not create a fresh workspace descriptor: state=%+v doc=%+v exists=%v", state, doc, ok)
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
