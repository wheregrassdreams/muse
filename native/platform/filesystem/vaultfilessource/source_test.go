package vaultfilessource_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	vaultfilessource "muse/platform/filesystem/vaultfilessource"
	vaultfiles "muse/vault/files"
)

func writeFile(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCatalogFilesystemSourceFilteringRevisionAndIncrementalUpdate(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.png", "nested/photo.jpg", ".hidden/secret.png", "visible/.secret.png"} {
		writeFile(t, root, name)
	}
	source, err := vaultfilessource.New(root)
	if err != nil {
		t.Fatal(err)
	}
	catalog := vaultfiles.New(source)
	if err := catalog.Rebuild(); err != nil {
		t.Fatal(err)
	}
	want := []string{"a.png", "nested/photo.jpg"}
	if got := catalog.Paths(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Paths() = %#v, want %#v", got, want)
	}
	initialRevision := catalog.Revision()
	if err := catalog.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if catalog.Revision() != initialRevision {
		t.Fatalf("unchanged rebuild changed revision: %d -> %d", initialRevision, catalog.Revision())
	}
	newPath := writeFile(t, root, "added/image.webp")
	catalog.Update(newPath)
	if got := catalog.Revision(); got != initialRevision+1 {
		t.Fatalf("revision after add = %d, want %d", got, initialRevision+1)
	}
	if got := catalog.FindImage("image.webp"); got != newPath {
		t.Fatalf("FindImage = %q, want %q", got, newPath)
	}
}

func TestSourceWalkPruningAndSymlinkBoundaries(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := writeFile(t, outside, "outside.png")
	writeFile(t, root, "visible/photo.png")
	writeFile(t, root, ".private/nested/secret.png")
	if err := os.Symlink(outside, filepath.Join(root, "linked-dir")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(root, "linked-file.png")); err != nil {
		t.Fatal(err)
	}
	source, err := vaultfilessource.New(root)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	err = source.Walk(func(entry vaultfiles.Entry) error {
		if entry.Kind == vaultfiles.EntryDirectory && entry.Path == ".private" {
			return vaultfiles.ErrSkipDir
		}
		paths = append(paths, entry.Path)
		if filepath.IsAbs(entry.Path) || strings.Contains(entry.Path, "\\") {
			t.Errorf("walk emitted non-relative path %q", entry.Path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(paths, ","), ".private/") || !reflect.DeepEqual(paths, []string{"linked-dir", "linked-file.png", "visible", "visible/photo.png"}) {
		t.Fatalf("walk paths = %#v", paths)
	}
	if _, err := source.Stat("linked-dir/outside.png"); err == nil {
		t.Fatal("Stat accepted a path through a symlinked directory")
	}
	for _, escaped := range []string{"../outside.png", "visible/../../outside.png", filepath.Join(root, "visible/photo.png"), `visible\\..\\outside.png`} {
		if _, err := source.Stat(escaped); err == nil {
			t.Errorf("Stat accepted escaped path %q", escaped)
		}
	}
	entry, err := source.Stat("linked-file.png")
	if err != nil || entry.Kind != vaultfiles.EntrySymlink {
		t.Fatalf("final symlink Stat = %+v, %v", entry, err)
	}
	catalog := vaultfiles.New(source)
	if err := catalog.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if got := catalog.Paths(); !reflect.DeepEqual(got, []string{"visible/photo.png"}) {
		t.Fatalf("catalog followed symlink or included hidden path: %#v", got)
	}
	catalog.Update(filepath.Join(root, "linked-dir", "outside.png"))
	catalog.Update(filepath.Join(root, "linked-file.png"))
	if got := catalog.Paths(); !reflect.DeepEqual(got, []string{"visible/photo.png"}) {
		t.Fatalf("incremental update crossed symlink boundary: %#v", got)
	}
}

func TestSourceRejectsMissingRootAndInvalidWalkCallback(t *testing.T) {
	source, err := vaultfilessource.New(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Walk(func(vaultfiles.Entry) error { return nil }); err == nil {
		t.Fatal("Walk accepted missing root")
	}
	if err := source.Walk(nil); err == nil {
		t.Fatal("Walk accepted nil callback")
	}
}
