package nativeui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceStateRoundTripAndRejectMissingPaths(t *testing.T) {
	config := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", config)
	vault := t.TempDir()
	note := filepath.Join(vault, "Note.md")
	if err := os.WriteFile(note, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := workspaceState{Root: vault, Path: note}
	if err := saveWorkspaceState(want); err != nil {
		t.Fatal(err)
	}
	if got := loadWorkspaceState(); got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
	if err := os.Remove(note); err != nil {
		t.Fatal(err)
	}
	got := loadWorkspaceState()
	if got.Root != vault || got.Path != "" {
		t.Fatalf("missing note should keep vault and clear document: %+v", got)
	}
}

func TestWorkspaceStateRejectsDocumentOutsideVault(t *testing.T) {
	config, vault := t.TempDir(), t.TempDir()
	outside := filepath.Join(t.TempDir(), "Else.md")
	if err := os.WriteFile(outside, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MUSE_CONFIG_DIR", config)
	if err := saveWorkspaceState(workspaceState{Root: vault, Path: outside}); err != nil {
		t.Fatal(err)
	}
	if got := loadWorkspaceState(); got.Path != "" || got.Root != vault {
		t.Fatalf("unsafe state = %+v", got)
	}
}

func TestWorkspaceStateRestoresVaultDocumentAndCaret(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	vault := t.TempDir()
	note := filepath.Join(vault, "Note.md")
	if err := os.WriteFile(note, []byte("first\nsecond\nthird"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveWorkspaceState(workspaceState{Root: vault, Path: note, Offset: 8}); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.restoreWorkspaceState()
	if a.root != vault || a.path != note || a.source != "first\nsecond\nthird" {
		t.Fatalf("restored vault/document = root %q path %q source %q", a.root, a.path, a.source)
	}
	if got := a.ed.SelectionRange(); got.Start != 8 || got.End != 8 {
		t.Fatalf("restored selection = %+v, want caret at 8", got)
	}
}
