package recentstore

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	workspaceapp "muse/application/workspace"
)

func TestStoreRoundTripsRecentEntriesAndChecksExistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, []byte("note"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(filepath.Join(dir, "config", "recent.json"))
	want := []workspaceapp.RecentEntry{{Path: path}, {Path: dir, IsDir: true}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %+v, %v; want %+v", got, err, want)
	}
	if !store.Exists(path) || store.Exists(filepath.Join(dir, "missing")) {
		t.Fatal("Exists did not reflect filesystem state")
	}
}

func TestStoreMissingFileLoadsEmpty(t *testing.T) {
	entries, err := New(filepath.Join(t.TempDir(), "missing.json")).Load()
	if err != nil || len(entries) != 0 {
		t.Fatalf("Load missing = %+v, %v", entries, err)
	}
}
