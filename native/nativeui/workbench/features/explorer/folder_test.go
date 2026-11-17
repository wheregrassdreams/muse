package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestFolderFirstClickAndWorkspaceReset(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "child")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	a := newFileTreeTestApp(t, dir)
	a.setWorkspace(dir)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	clickTreeRow(t, tt, "child")
	if !a.fileTreeState.IsExpanded(sub) {
		t.Fatal("first folder click did not expand")
	}
	a.setWorkspace(dir)
	if a.fileTreeState.IsExpanded(sub) {
		t.Fatal("workspace switch retained expanded folder state")
	}
	tt.Frame()
	clickTreeRow(t, tt, "child")
	if !a.fileTreeState.IsExpanded(sub) {
		t.Fatal("folder did not expand after workspace reset")
	}
}
