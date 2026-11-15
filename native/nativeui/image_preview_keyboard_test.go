package nativeui

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestImagePreviewOwnsEscapeBeforeFileTreeActions(t *testing.T) {
	root := keyTestWorkspace(t)
	a := New()
	a.setWorkspace(root)
	a.focusPanel(PanelIDFiles)
	a.openImagePreview(filepath.Join(root, "missing.png"))
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	a.lastPanelAction = ""

	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.imagePreviewOpen() {
		t.Fatal("Escape did not close the image preview")
	}
	if a.lastPanelAction != "" {
		t.Fatalf("Escape also dispatched a file-tree action %q", a.lastPanelAction)
	}
}
