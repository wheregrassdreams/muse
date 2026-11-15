package agentworkspace

import (
	"os"
	"path/filepath"
	"testing"
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
