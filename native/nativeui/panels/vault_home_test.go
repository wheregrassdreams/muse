package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestSwitchingWorkspaceClearsDocumentAndShowsVaultActions(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.applyOpen(filepath.Join(t.TempDir(), "Old.md"), "old vault content")
	a.setWorkspace(root)

	if a.path != "" || a.source != "" || a.dirty {
		t.Fatalf("workspace switch retained prior document: path=%q source=%q dirty=%v", a.path, a.source, a.dirty)
	}
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText(locales.DocumentNew()) || !tt.HasText(locales.FilesOpenFileMenu()) {
		t.Fatalf("vault landing actions missing: %q", tt.Texts())
	}
}

func TestSettingSameWorkspaceKeepsCurrentDocument(t *testing.T) {
	root := t.TempDir()
	a := New()
	a.setWorkspace(root)
	a.applyOpen(filepath.Join(root, "Current.md"), "keep me")
	a.setWorkspace(root)
	if a.path != filepath.Join(root, "Current.md") || a.source != "keep me" {
		t.Fatalf("reselecting same workspace cleared document: path=%q source=%q", a.path, a.source)
	}
}
