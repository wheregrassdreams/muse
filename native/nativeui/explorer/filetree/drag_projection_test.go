package filetree

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestTreeMultiSelectionProjectionShowsCountAndStackedIcon(t *testing.T) {
	handle := NewHandle()
	handle.SetSelection([]string{"/vault/a.md", "/vault/b.md"})
	handle.SetDragOperationDisplay(DragOperationDisplay{Kind: DragOperationMoveToFolder, TargetLabel: "Folder"})
	model := BuildDragProjection(handle, []Row{
		{Path: "/vault/a.md"}, {Path: "/vault/b.md"}, {Path: "/vault/Folder", Directory: true},
	}, DragProjectionInput{
		ItemName:    "b.md",
		IsDirectory: func(path string) bool { return path == "/vault/Folder" },
	})
	if model.SelectedCount != 2 || !model.SelectedFilesOnly || model.ItemKind != File || model.OperationKind != DragOperationMoveToFolder {
		t.Fatalf("drag model = %+v", model)
	}
	handle.SetDragPreview(DragPreviewState{Active: true, Frames: 3, ItemID: "b.md"})
	tester := ui.NewTester(func(c *ui.Context) { RenderDragProjection(c, handle, model) }, 480, 240)
	tester.Frame()
	if !tester.HasText(locales.FilesSelectedFileCount(2)) || !tester.HasText(locales.TreeDragProjectionOperationLabel("b.md")) {
		t.Fatalf("multi-selection projection text = %v", tester.Texts())
	}
}

func TestTreeTrashHoverKeepsCompactTwoLineProjection(t *testing.T) {
	handle := NewHandle()
	handle.SetSelection([]string{"/vault/note.md"})
	handle.SetDragOperationDisplay(DragOperationDisplay{Kind: DragOperationMoveToTrash})
	handle.SetDragPreview(DragPreviewState{Active: true, ItemID: "/vault/note.md"})
	model := BuildDragProjection(handle, []Row{{Path: "/vault/note.md"}}, DragProjectionInput{ItemName: "note.md"})
	tester := ui.NewTester(func(c *ui.Context) { RenderDragProjection(c, handle, model) }, 480, 240)
	tester.Frame()
	if !tester.HasText(locales.TreeDragProjectionLabel("/vault/note.md")) || !tester.HasText(locales.TreeDragProjectionOperationLabel("/vault/note.md")) {
		t.Fatalf("projection lost trash operation text: %v", tester.Texts())
	}
}

func TestDragProjectionSurvivesCollapsedFolderExpansion(t *testing.T) {
	handle := NewHandle()
	handle.SetSelection([]string{"/vault/folder", "/vault/folder/note.md"})
	handle.SetDragOperationDisplay(DragOperationDisplay{Kind: DragOperationMoveToFolder, TargetLabel: "Other"})
	rows := []Row{{Path: "/vault/folder", Directory: true}, {Path: "/vault/folder/note.md"}}
	before := BuildDragProjection(handle, rows, DragProjectionInput{ItemName: "folder", ItemDirectory: true})
	handle.SetExpanded("/vault/folder", true)
	after := BuildDragProjection(handle, rows[:1], DragProjectionInput{ItemName: "folder", ItemDirectory: true})
	if before.SelectedCount != 2 || after.SelectedCount != 2 || after.OperationKind != DragOperationMoveToFolder {
		t.Fatalf("drag projection changed as rows were collapsed/expanded: before=%+v after=%+v", before, after)
	}
}
