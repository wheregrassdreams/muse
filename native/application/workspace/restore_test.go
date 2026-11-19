package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"muse/events"
	"muse/platform/filesystem/workspacestore"
	"muse/workspace"
)

type memoryWorkspaceRepository struct {
	state workspace.State
	err   error
}

func (r *memoryWorkspaceRepository) Load() (workspace.State, error) { return r.state, r.err }
func (r *memoryWorkspaceRepository) Save(state workspace.State) error {
	if r.err != nil {
		return r.err
	}
	r.state = state
	return nil
}

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
	plan, err := NewRestorer(store, dispatcher).Restore()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.State, want) || !reflect.DeepEqual(received, want) {
		t.Fatalf("restore plan = %+v event = %+v, want state %+v", plan, received, want)
	}
	if plan.DefaultTab.Decision != UseRestoredTabs || plan.ActiveTab != tabID || plan.ActivePane != paneID || len(plan.Documents) != 1 || plan.Documents[0].Path != path || !plan.Documents[0].Focused {
		t.Fatalf("restore document plan = %+v", plan)
	}
}

func TestRestorePlanFiltersForeignDocumentsAndPreservesStableOrder(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.md")
	second := filepath.Join(root, "second.md")
	other := filepath.Join(t.TempDir(), "outside.md")
	state := workspace.State{
		Version: 1, VaultPath: filepath.Join(root, "notes", ".."), ActiveTab: "removed",
		Tabs: []workspace.Tab{
			{ID: "removed", ActivePaneID: "foreign", Layout: workspace.Leaf(workspace.Pane{ID: "foreign", Document: workspace.Document{Path: other}})},
			{ID: "survivor", ActivePaneID: "foreign-pane", Layout: workspace.Branch(workspace.Horizontal, 0.3,
				workspace.Leaf(workspace.Pane{ID: "foreign-pane", Document: workspace.Document{Path: other}}),
				workspace.Leaf(workspace.Pane{ID: "first-pane", Document: workspace.Document{Path: first, CursorOffset: 7}}),
			)},
			{ID: "last", ActivePaneID: "last-pane", Layout: workspace.Leaf(workspace.Pane{ID: "last-pane", Document: workspace.Document{Path: second, CursorOffset: 2}})},
		},
	}
	plan, err := makeRestorePlan(state)
	if err != nil {
		t.Fatal(err)
	}
	if plan.VaultPath != root || plan.State.ActiveTab != "survivor" || plan.ActiveTab != "survivor" || plan.ActivePane != "first-pane" {
		t.Fatalf("restore selection = %+v", plan)
	}
	if len(plan.State.Tabs) != 2 || plan.State.Tabs[0].ID != "survivor" || plan.State.Tabs[1].ID != "last" {
		t.Fatalf("tab order = %+v", plan.State.Tabs)
	}
	if plan.State.Tabs[0].Layout.Pane == nil || plan.State.Tabs[0].Layout.Pane.ID != "first-pane" {
		t.Fatalf("single owned child did not collapse into the restored layout: %+v", plan.State.Tabs[0].Layout)
	}
	wantPaths := []string{first, second}
	for i, want := range wantPaths {
		if plan.Documents[i].Path != want {
			t.Fatalf("document order = %+v, want paths %v", plan.Documents, wantPaths)
		}
	}
	if !plan.Documents[0].Focused || plan.Documents[1].Focused {
		t.Fatalf("focused document markers = %+v", plan.Documents)
	}
}

func TestRestorePlanRequestsDefaultDocumentWhenNoOwnedNotesRemain(t *testing.T) {
	state := workspace.State{Version: 1, VaultPath: t.TempDir()}
	plan, err := makeRestorePlan(state)
	if err != nil {
		t.Fatal(err)
	}
	if plan.DefaultTab.Decision != CreateBlankTab || plan.DefaultTab.TabID != DefaultTabID || plan.DefaultTab.PaneID != DefaultPaneID || len(plan.Documents) != 0 || plan.ActiveTab != "" || plan.ActivePane != "" {
		t.Fatalf("empty restore plan = %+v", plan)
	}
}

func TestRestoreRejectsInvalidPersistedStateBeforePublishing(t *testing.T) {
	dispatcher := events.New[workspace.WorkspaceRestored]()
	received := 0
	dispatcher.Subscribe(func(workspace.WorkspaceRestored) { received++ })
	store := &memoryWorkspaceRepository{state: workspace.State{Version: 99}}
	if _, err := NewRestorer(store, dispatcher).Restore(); !errors.Is(err, workspace.ErrInvalidState) {
		t.Fatalf("Restore error=%v, want invalid state", err)
	}
	if received != 0 {
		t.Fatalf("invalid state published %d restore events", received)
	}
}

func TestPersistRejectsDocumentsOutsideTheirVault(t *testing.T) {
	repository := &memoryWorkspaceRepository{}
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	state := workspace.State{
		Version: 1, VaultPath: root, ActiveTab: "main",
		Tabs: []workspace.Tab{{ID: "main", ActivePaneID: "pane", Layout: workspace.Leaf(workspace.Pane{
			ID: "pane", Document: workspace.Document{Path: outside},
		})}},
	}
	if err := NewRestorer(repository, nil).Persist(state); !errors.Is(err, ErrDocumentOutsideVault) {
		t.Fatalf("Persist error=%v, want outside-vault error", err)
	}
	if len(repository.state.Tabs) != 0 {
		t.Fatalf("rejected state was persisted: %+v", repository.state)
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
