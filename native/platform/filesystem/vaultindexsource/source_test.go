package vaultindexsource_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muse/platform/filesystem/vaultindexsource"
	vaultindex "muse/vault/index"
)

func writeFile(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWalkUsesRelativePathsAndHonorsDirectoryPruning(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "visible/note.md", "visible")
	writeFile(t, root, ".private/nested/secret.md", "secret")
	source, err := vaultindexsource.New(root)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	err = source.Walk(func(entry vaultindex.Entry) error {
		if entry.Kind == vaultindex.EntryDirectory && entry.Path == ".private" {
			return vaultindex.ErrSkipDir
		}
		paths = append(paths, entry.Path)
		if filepath.IsAbs(entry.Path) || strings.Contains(entry.Path, "\\") {
			t.Errorf("entry path is not slash-relative: %q", entry.Path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(paths, ",")
	if strings.Contains(joined, ".private/") || !strings.Contains(joined, "visible/note.md") {
		t.Fatalf("walk paths = %v", paths)
	}
}

func TestStatReadAndPathBoundary(t *testing.T) {
	root := t.TempDir()
	contentPath := writeFile(t, root, "notes/a.md", "hello")
	if err := os.Mkdir(filepath.Join(root, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	source, err := vaultindexsource.New(root)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := source.Stat("notes/a.md")
	if err != nil || entry.Path != "notes/a.md" || entry.Kind != vaultindex.EntryRegular {
		t.Fatalf("regular Stat = %+v, %v", entry, err)
	}
	entry, err = source.Stat("folder")
	if err != nil || entry.Kind != vaultindex.EntryDirectory {
		t.Fatalf("directory Stat = %+v, %v", entry, err)
	}
	got, err := source.Read("notes/a.md")
	if err != nil || string(got) != "hello" {
		t.Fatalf("Read = %q, %v", got, err)
	}
	for _, path := range []string{"../outside.md", filepath.Join(root, "notes/a.md"), "notes/../../outside.md"} {
		if _, err := source.Read(path); err == nil {
			t.Errorf("Read accepted out-of-root path %q", path)
		}
		if _, err := source.Stat(path); err == nil {
			t.Errorf("Stat accepted out-of-root path %q", path)
		}
	}
	if _, err := source.Read(filepath.Base(contentPath) + "/missing"); err == nil {
		t.Fatal("Read accepted a missing path")
	}
}

func TestWalkDoesNotFollowSymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideNote := writeFile(t, outside, "outside.md", "outside-token")
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	fileLink := filepath.Join(root, "linked-file.md")
	if err := os.Symlink(outsideNote, fileLink); err != nil {
		t.Fatal(err)
	}
	source, err := vaultindexsource.New(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []vaultindex.Entry
	if err := source.Walk(func(entry vaultindex.Entry) error {
		got = append(got, entry)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != "linked" || got[0].Kind != vaultindex.EntrySymlink || got[1].Path != "linked-file.md" || got[1].Kind != vaultindex.EntrySymlink {
		t.Fatalf("symlink walk = %+v", got)
	}
	if _, err := source.Stat("linked/outside.md"); err == nil {
		t.Fatal("Stat accepted a path through a symlinked directory")
	}
	if _, err := source.Read("linked/outside.md"); err == nil {
		t.Fatal("Read followed a symlinked directory")
	}
	entry, err := source.Stat("linked-file.md")
	if err != nil || entry.Kind != vaultindex.EntrySymlink {
		t.Fatalf("stat symlink file = %+v, %v", entry, err)
	}
	if _, err := source.Read("linked-file.md"); err == nil {
		t.Fatal("Read followed a symlinked file")
	}
	idx := vaultindex.New(source)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.Document("linked/outside.md"); ok {
		t.Fatal("index followed a symlinked directory")
	}
}

func TestIndexRebuildAndIncrementalUpdateUseSource(t *testing.T) {
	root := t.TempDir()
	note := writeFile(t, root, "notes/a.md", "first-token")
	source, err := vaultindexsource.New(root)
	if err != nil {
		t.Fatal(err)
	}
	idx := vaultindex.New(source)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if got := idx.Query("first-token"); len(got) != 1 {
		t.Fatalf("initial query = %+v", got)
	}
	if err := os.WriteFile(note, []byte("second-token"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(note); err != nil {
		t.Fatal(err)
	}
	if len(idx.Query("first-token")) != 0 || len(idx.Query("second-token")) != 1 {
		t.Fatal("incremental update did not replace the note")
	}
	if err := os.Remove(note); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(note); err != nil {
		t.Fatal(err)
	}
	if len(idx.Query("second-token")) != 0 {
		t.Fatal("missing-file update did not remove the note")
	}
}
