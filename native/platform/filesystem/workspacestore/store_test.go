package workspacestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"muse/workspace"
)

func TestStoreRoundTripsTabsSplitPanesAndFocus(t *testing.T) {
	vault := t.TempDir()
	left := writeNote(t, vault, "Left.md")
	right := writeNote(t, vault, "Right.md")
	other := writeNote(t, vault, "Other.md")
	state := workspace.State{
		Version: 1, VaultPath: vault, ActiveTab: "work",
		Tabs: []workspace.Tab{
			{ID: "work", ActivePaneID: "right", Layout: workspace.Branch(workspace.Horizontal, 0.38,
				workspace.Leaf(workspace.Pane{ID: "left", Document: workspace.Document{Path: left, CursorOffset: 3}}),
				workspace.Leaf(workspace.Pane{ID: "right", Document: workspace.Document{Path: right, CursorOffset: 12}}),
			)},
			{ID: "other", ActivePaneID: "only", Layout: workspace.Leaf(workspace.Pane{ID: "only", Document: workspace.Document{Path: other}})},
		},
	}
	store := New(filepath.Join(t.TempDir(), "workspace.json"))
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil || !reflect.DeepEqual(got, state) {
		t.Fatalf("round trip = %#v, %v; want %#v", got, err, state)
	}
	if mode := fileMode(t, store.path); mode.Perm() != 0o600 {
		t.Fatalf("snapshot permissions = %o, want 600", mode.Perm())
	}
}

func TestLoadMigratesSingleDocumentSnapshot(t *testing.T) {
	vault := t.TempDir()
	note := writeNote(t, vault, "Note.md")
	path := filepath.Join(t.TempDir(), "workspace.json")
	legacy, err := json.Marshal(struct {
		Root   string `json:"root"`
		Path   string `json:"path"`
		Offset int    `json:"offset"`
	}{vault, note, 7})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := New(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	doc, ok := got.ActiveDocument()
	if !ok || got.VaultPath != vault || doc.Path != note || doc.CursorOffset != 7 || len(got.Tabs) != 1 {
		t.Fatalf("migrated snapshot = %#v, active document = %#v, %v", got, doc, ok)
	}
}

func TestLoadDropsMissingPaneAndCollapsesSplit(t *testing.T) {
	vault := t.TempDir()
	kept := writeNote(t, vault, "Kept.md")
	missing := filepath.Join(vault, "Gone.md")
	state := workspace.State{
		Version: 1, VaultPath: vault, ActiveTab: "main",
		Tabs: []workspace.Tab{{ID: "main", ActivePaneID: "gone", Layout: workspace.Branch(workspace.Vertical, 0.5,
			workspace.Leaf(workspace.Pane{ID: "kept", Document: workspace.Document{Path: kept}}),
			workspace.Leaf(workspace.Pane{ID: "gone", Document: workspace.Document{Path: missing}}),
		)}},
	}
	path := filepath.Join(t.TempDir(), "workspace.json")
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := New(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tabs) != 1 || got.Tabs[0].ActivePaneID != "kept" || got.Tabs[0].Layout.Pane == nil || got.Tabs[0].Layout.Pane.Document.Path != kept {
		t.Fatalf("sanitized snapshot = %#v", got)
	}
}

func TestStoreRejectsDocumentOutsideVault(t *testing.T) {
	vault, outsideDir := t.TempDir(), t.TempDir()
	note := writeNote(t, outsideDir, "Else.md")
	state := workspace.State{
		Version: 1, VaultPath: vault, ActiveTab: "main",
		Tabs: []workspace.Tab{{ID: "main", ActivePaneID: "pane", Layout: workspace.Leaf(workspace.Pane{ID: "pane", Document: workspace.Document{Path: note}})}},
	}
	if err := New(filepath.Join(t.TempDir(), "workspace.json")).Save(state); err == nil {
		t.Fatal("Save accepted a document outside the vault")
	}
}

func writeNote(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte("hello workspace"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}
