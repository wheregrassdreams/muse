package nativeui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyRelativeAndAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.md")
	if err := os.WriteFile(file, []byte("# One\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(dir)
	var copied string
	old := clipboardWrite
	clipboardWrite = func(s string) { copied = s }
	defer func() { clipboardWrite = old }()
	a.copyPath(file, true)
	if copied != "one.md" {
		t.Fatalf("relative clipboard = %q", copied)
	}
	a.copyPath(file, false)
	if copied != file {
		t.Fatalf("absolute clipboard = %q, want %q", copied, file)
	}
}
