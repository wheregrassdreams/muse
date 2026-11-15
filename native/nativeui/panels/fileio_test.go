package nativeui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicKeepsTheOldFileWhole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("new")); err != nil {
		t.Fatalf("write: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "new" {
		t.Fatalf("content %q, err %v", b, err)
	}
	// The mode the note had survives the save.
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, err %v", st.Mode(), err)
	}
	// No temporary file is left beside it.
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 || ents[0].Name() != "note.md" {
		t.Fatalf("directory holds %v", ents)
	}
}

func TestWriteFileAtomicCreatesANewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.md")
	if err := writeFileAtomic(path, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "hi" {
		t.Fatalf("content %q, err %v", b, err)
	}
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v, err %v", st.Mode(), err)
	}
}
