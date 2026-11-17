package filetree

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestProjectChildrenFiltersAndOrdersCatalogEntries(t *testing.T) {
	entries := []CatalogEntry{
		{Path: "z.md", Kind: CatalogFile},
		{Path: "notes", Kind: CatalogDirectory},
		{Path: "Archive", Kind: CatalogDirectory},
		{Path: "image.png", Kind: CatalogFile},
		{Path: ".hidden", Kind: CatalogFile},
		{Path: "node_modules", Kind: CatalogDirectory},
		{Path: "a.md", Kind: CatalogFile},
	}
	got := ProjectChildren("/vault", entries, func(path string) bool {
		return filepath.Ext(path) == ".md"
	})
	want := []string{
		filepath.Join("/vault", "Archive"),
		filepath.Join("/vault", "notes"),
		filepath.Join("/vault", "a.md"),
		filepath.Join("/vault", "z.md"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ProjectChildren() = %v, want %v", got, want)
	}
}

func TestSelectionSummaryUsesCallerCatalogFacts(t *testing.T) {
	isDirectory := func(path string) bool { return path == "folder" }
	for _, tc := range []struct {
		name      string
		paths     []string
		lookup    func(string) bool
		count     int
		filesOnly bool
	}{
		{name: "empty"},
		{name: "files", paths: []string{"a.md", "b.md"}, lookup: isDirectory, count: 2, filesOnly: true},
		{name: "mixed", paths: []string{"a.md", "folder"}, lookup: isDirectory, count: 2},
		{name: "unknown", paths: []string{"a.md"}, count: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count, filesOnly := SelectionSummary(tc.paths, tc.lookup)
			if count != tc.count || filesOnly != tc.filesOnly {
				t.Fatalf("SelectionSummary() = %d/%v, want %d/%v", count, filesOnly, tc.count, tc.filesOnly)
			}
		})
	}
}

func TestProjectChildrenRejectsEscapingCatalogPaths(t *testing.T) {
	got := ProjectChildren("/vault", []CatalogEntry{
		{Path: "../outside", Kind: CatalogDirectory},
		{Path: "/absolute", Kind: CatalogFile},
		{Path: "safe.md", Kind: CatalogFile},
	}, nil)
	want := []string{filepath.Join("/vault", "safe.md")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ProjectChildren() = %v, want %v", got, want)
	}
}

func TestRenderDragProjectionShowsMultiSelectionAndOperation(t *testing.T) {
	model := DragProjection{
		ItemName: "a.md", ItemKind: File,
		SelectedCount: 3, SelectedFilesOnly: true,
		OperationKind:  DragOperationMoveToTrash,
		OperationLabel: (DragOperationDisplay{Kind: DragOperationMoveToTrash}).Label(),
	}
	state := newState()
	state.SetDragPreview(DragPreviewState{Active: true, ItemID: "notes/a.md"})
	tester := ui.NewTester(func(c *ui.Context) {
		RenderDragProjection(c, &Handle{state: state}, model)
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
	state := newState()
	tester := ui.NewTester(func(c *ui.Context) {
		RenderDragProjection(c, &Handle{state: state}, DragProjection{ItemName: "note.md"})
	}, 480, 240)
	tester.Frame()
	if tester.HasText(locales.TreeDragProjectionLabel("note.md")) {
		t.Fatalf("inactive drag projection rendered: %v", tester.Texts())
	}
}
