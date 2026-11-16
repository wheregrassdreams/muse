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
	a := newFileTreeTestApp(t, root)
	a.applyOpen(filepath.Join(t.TempDir(), "Old.md"), "old vault content")
	a.setWorkspace(root)

	if a.documentPath() != "" || a.documentSource() != "" || a.documentDirty() {
		t.Fatalf("workspace switch retained prior document: path=%q source=%q dirty=%v", a.documentPath(), a.documentSource(), a.documentDirty())
	}
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText(locales.DocumentNew()) || !tt.HasText(locales.FilesOpenFileMenu()) {
		t.Fatalf("vault landing actions missing: %q", tt.Texts())
	}
}

func TestSettingSameWorkspaceKeepsCurrentDocument(t *testing.T) {
	root := t.TempDir()
	a := newFileTreeTestApp(t, root)
	a.setWorkspace(root)
	a.applyOpen(filepath.Join(root, "Current.md"), "keep me")
	a.setWorkspace(root)
	if a.documentPath() != filepath.Join(root, "Current.md") || a.documentSource() != "keep me" {
		t.Fatalf("reselecting same workspace cleared document: path=%q source=%q", a.documentPath(), a.documentSource())
	}
}
