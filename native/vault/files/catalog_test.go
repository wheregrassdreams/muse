package vaultfiles

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
	c := New(root)
	if err := c.Rebuild(); err != nil {
		t.Fatal(err)
	}
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

func TestCatalogDuplicateSelectionAndDirectoryRemoval(t *testing.T) {
	root := t.TempDir()
	long := write(t, root, "longer/path/shared.png")
	short := write(t, root, "x/shared.png")
	write(t, root, "x/y/shared.png")
	c := New(root)
	if err := c.Rebuild(); err != nil {
		t.Fatal(err)
	}
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
	c := New(root)
	if err := c.Rebuild(); err != nil {
		t.Fatal(err)
	}
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
