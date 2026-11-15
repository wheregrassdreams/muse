package agentworkspace

import (
	"os"
	"path/filepath"
	"testing"

	"muse/application/agent"
)

func TestResolveWithinRootSupportsMissingTargets(t *testing.T) {
	root := t.TempDir()
	w, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := w.ResolveWithinRoot(filepath.Join(root, "new", "note.md"))
	if err != nil {
		t.Fatal(err)
	}
	canonicalRoot, _ := filepath.EvalSymlinks(root)
	if got != filepath.Join(canonicalRoot, "new", "note.md") {
		t.Fatalf("resolved path=%q", got)
	}
}

func TestResolveWithinRootRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	w, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.ReadFile(filepath.Join(root, "escape", "secret.md")); err == nil {
		t.Fatal("read through escaping symlink succeeded")
	}
	if _, err := w.ResolveWithinRoot(filepath.Join(root, "escape", "new.md")); err == nil {
		t.Fatal("missing target beneath escaping symlink succeeded")
	}
}

func TestCurrentDocumentProviderIsInjected(t *testing.T) {
	root := t.TempDir()
	w, err := New(root, func() (string, string) { return filepath.Join(root, "draft.md"), "unsaved" })
	if err != nil {
		t.Fatal(err)
	}
	path, source := w.CurrentDocument()
	if path != filepath.Join(root, "draft.md") || source != "unsaved" {
		t.Fatalf("current=(%q,%q)", path, source)
	}
}

func TestReadDirReturnsAgentOwnedEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "notes"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Home.md"), []byte("home"), 0600); err != nil {
		t.Fatal(err)
	}
	w, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := w.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("ReadDir returned %d entries, want 2", len(entries))
	}
	got := map[string]agent.Entry{}
	for _, entry := range entries {
		got[entry.Name] = entry
	}
	canonicalRoot := w.Root()
	if entry := got["notes"]; entry.Path != filepath.Join(canonicalRoot, "notes") || !entry.IsDir {
		t.Fatalf("directory entry = %#v", entry)
	}
	if entry := got["Home.md"]; entry.Path != filepath.Join(canonicalRoot, "Home.md") || entry.IsDir {
		t.Fatalf("file entry = %#v", entry)
	}
}
