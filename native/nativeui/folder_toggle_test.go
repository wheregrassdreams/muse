package nativeui

import (
	"github.com/egoist/mygo/ui"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFolderFirstClickAndWorkspaceReset(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "child")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	a := New()
	if a.folderToggled == nil {
		t.Fatal("constructor did not initialize toggle state")
	}
	a.setWorkspace(dir)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if err := tt.Click("child"); err != nil {
		t.Fatal(err)
	}
	if !a.expanded[sub] {
		t.Fatal("first folder click did not expand")
	}
	if a.folderToggled[sub].IsZero() {
		t.Fatal("toggle timestamp missing")
	}
	a.setWorkspace(dir)
	if len(a.folderToggled) != 0 {
		t.Fatal("workspace retained debounce state")
	}
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("child"); err != nil {
		t.Fatal(err)
	}
	if !a.expanded[sub] {
		t.Fatal("folder did not expand after workspace reset")
	}
}
