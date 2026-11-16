package vaultfiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyDirectoryPreservesModeAndSkipsNestedSymlinks(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o750); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(source, "nested")
	if err := os.Mkdir(nested, 0o710); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(nested, "note.md")
	if err := os.WriteFile(file, []byte("body"), 0o640); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(source, "linked.md")
	if err := os.Symlink(file, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	destination := filepath.Join(root, "copy")
	if err := New(nil).Copy(source, destination); err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(destination, "nested", "note.md")
	if body, err := os.ReadFile(copied); err != nil || string(body) != "body" {
		t.Fatalf("copied file = %q, %v", body, err)
	}
	info, err := os.Stat(copied)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("copied file permissions = %v, %v", info, err)
	}
	if _, err := os.Lstat(filepath.Join(destination, "linked.md")); !os.IsNotExist(err) {
		t.Fatalf("copied symlink exists: %v", err)
	}
}

func TestTrashRequiresPlatformCapability(t *testing.T) {
	if err := New(nil).Trash("missing.md"); err == nil {
		t.Fatal("Trash without OS capability succeeded")
	}
}

func TestCreateFileIsExclusiveAndPreservesRequestedContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	adapter := New(nil)
	if err := adapter.CreateFile(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := adapter.CreateFile(path, []byte("second")); !os.IsExist(err) {
		t.Fatalf("duplicate CreateFile error = %v, want exist", err)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != "first" {
		t.Fatalf("existing file content = %q, %v", body, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("new file mode = %v, %v", info, err)
	}
}

func TestCreateDirectoryCreatesSingleDirectoryAndRejectsCollision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "folder")
	adapter := New(nil)
	if err := adapter.CreateDirectory(path); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() || info.Mode().Perm() != 0o755 {
		t.Fatalf("created folder = %v, %v", info, err)
	}
	if err := adapter.CreateDirectory(path); !os.IsExist(err) {
		t.Fatalf("duplicate CreateDirectory error = %v, want exist", err)
	}
}
