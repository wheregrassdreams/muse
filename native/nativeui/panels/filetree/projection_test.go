package filetree

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestRenderDragProjectionShowsMultiSelectionAndOperation(t *testing.T) {
	model := DragProjection{
		Active: true, ItemID: "notes/a.md", ItemName: "a.md", ItemKind: File,
		SelectedCount: 3, SelectedFilesOnly: true,
		OperationID: "move-to-trash", OperationLabel: locales.FilesMoveToTrash(),
	}
	tester := ui.NewTester(func(c *ui.Context) {
		RenderDragProjection(c, model)
	}, 480, 240)
	tester.Frame()
	if !tester.HasText(locales.TreeDragProjectionLabel("notes/a.md")) {
		t.Fatalf("drag card label missing: %v", tester.Texts())
	}
	if !tester.HasText(locales.FilesSelectedFileCount(3)) {
		t.Fatalf("selected file count missing: %v", tester.Texts())
	}
	if !tester.HasText(locales.TreeDragProjectionOperationLabel("notes/a.md")) {
		t.Fatalf("operation label missing: %v", tester.Texts())
	}
}

func TestRenderDragProjectionOmitsInactiveCard(t *testing.T) {
	tester := ui.NewTester(func(c *ui.Context) {
		RenderDragProjection(c, DragProjection{ItemID: "note.md", ItemName: "note.md"})
	}, 480, 240)
	tester.Frame()
	if tester.HasText(locales.TreeDragProjectionLabel("note.md")) {
		t.Fatalf("inactive drag projection rendered: %v", tester.Texts())
	}
}
