package files

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func write(t *testing.T, root, name string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCatalogRebuildAndIncrementalUpdates(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.png", "nested/photo.jpg", "nested/anim.gif", "nested/modern.webp", "note.md"} {
		write(t, root, name)
	}
	write(t, root, ".hidden/secret.png")
	write(t, root, "visible/.secret.png")
	c := catalogForTest(t, root)
	want := []string{"a.png", "nested/anim.gif", "nested/modern.webp", "nested/photo.jpg", "note.md"}
	if got := c.Paths(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Paths() = %#v, want %#v", got, want)
	}
	if got := c.ImagePaths(); !reflect.DeepEqual(got, want[:4]) {
		t.Fatalf("ImagePaths() = %#v", got)
	}
	before := c.Revision()
	p := write(t, root, "deep/new.png")
	c.Update(p)
	if c.Revision() != before+1 {
		t.Fatalf("revision after add = %d, want %d", c.Revision(), before+1)
	}
	if got := c.FindImage("new.png"); got != p {
		t.Fatalf("FindImage = %q, want %q", got, p)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	c.Remove(p)
	if got := c.FindImage("new.png"); got != "" {
		t.Fatalf("removed image still resolves: %q", got)
	}
}

func TestCatalogChildrenAndEntryIncludeDirectoriesWithoutRescanning(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, root, "notes/child.md")
	write(t, root, "root.md")
	catalog := catalogForTest(t, root)

	got := catalog.Children("")
	want := []Entry{{Path: "empty", Kind: EntryDirectory}, {Path: "notes", Kind: EntryDirectory}, {Path: "root.md", Kind: EntryRegular}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("root children = %+v, want %+v", got, want)
	}
	if got := catalog.Children("notes"); !reflect.DeepEqual(got, []Entry{{Path: "notes/child.md", Kind: EntryRegular}}) {
		t.Fatalf("notes children = %+v", got)
	}
	if got, ok := catalog.Entry("empty"); !ok || got.Kind != EntryDirectory {
		t.Fatalf("empty directory entry = %+v, %v", got, ok)
	}
	if got, ok := catalog.Entry("root.md"); !ok || got.Kind != EntryRegular {
		t.Fatalf("file entry = %+v, %v", got, ok)
	}
	if _, ok := catalog.Entry("../root.md"); ok {
		t.Fatal("catalog accepted a path outside the vault")
	}
}

func TestCatalogUpdateTracksDirectoryKindAndChildren(t *testing.T) {
	root := t.TempDir()
	catalog := catalogForTest(t, root)
	directory := filepath.Join(root, "new")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	catalog.Update(directory)
	if got, ok := catalog.Entry("new"); !ok || got.Kind != EntryDirectory {
		t.Fatalf("new directory entry = %+v, %v", got, ok)
	}
	file := filepath.Join(directory, "note.md")
	write(t, root, "new/note.md")
	catalog.Update(file)
	if got := catalog.Children("new"); !reflect.DeepEqual(got, []Entry{{Path: "new/note.md", Kind: EntryRegular}}) {
		t.Fatalf("new directory children = %+v", got)
	}
	if err := os.RemoveAll(directory); err != nil {
		t.Fatal(err)
	}
	catalog.Remove(directory)
	if got := catalog.Children(""); len(got) != 0 {
		t.Fatalf("root children after directory removal = %+v", got)
	}
}

func TestCatalogDuplicateSelectionAndDirectoryRemoval(t *testing.T) {
	root := t.TempDir()
	long := write(t, root, "longer/path/shared.png")
	short := write(t, root, "x/shared.png")
	write(t, root, "x/y/shared.png")
	c := catalogForTest(t, root)
	if got := c.FindImage("SHARED.PNG"); got != short {
		t.Fatalf("duplicate selection = %q, want shortest %q (long was %q)", got, short, long)
	}
	c.Remove(filepath.Join(root, "x"))
	if got := c.FindImage("shared.png"); got != long {
		t.Fatalf("directory removal left wrong match: %q, want %q", got, long)
	}
}

func TestCatalogRejectsMissingOutsideHiddenAndSymlinkPaths(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	p := write(t, outside, "outside.png")
	write(t, root, "visible/.secret.png")
	c := catalogForTest(t, root)
	c.Update(p)
	c.Update(filepath.Join(root, ".hidden", "missing.png"))
	c.Update(filepath.Join(root, "visible", ".secret.png"))
	if err := os.Symlink(outside, filepath.Join(root, "linked-dir")); err == nil {
		c.Update(filepath.Join(root, "linked-dir", "outside.png"))
	}
	if got := c.Paths(); len(got) != 0 {
		t.Fatalf("unexpected paths: %#v", got)
	}
	if err := os.Symlink(p, filepath.Join(root, "linked.png")); err == nil {
		c.Update(filepath.Join(root, "linked.png"))
		if got := c.Paths(); len(got) != 0 {
			t.Fatalf("symlink entered catalog: %#v", got)
		}
	}
}
