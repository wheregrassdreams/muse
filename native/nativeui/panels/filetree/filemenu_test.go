package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestFileMenuListsCopyItems(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.md")
	if err := os.WriteFile(file, []byte("# One\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(dir)
	old := clipboardWrite
	copied := ""
	clipboardWrite = func(s string) { copied = s }
	defer func() { clipboardWrite = old }()

	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	item, ok := tt.Find("one.md")
	if !ok {
		t.Fatal("tree file missing")
	}
	tt.RightClickAt(item.X+4, item.Y+item.H/2)
	items := tt.Menu()
	if len(items) == 0 {
		t.Fatal("context menu is empty")
	}
	if err := tt.ChooseMenuItem("复制相对路径"); err != nil {
		t.Fatalf("choose copy relative: %v", err)
	}
	if copied != "one.md" {
		t.Fatalf("copied %q, want one.md", copied)
	}
}

func TestFileTreeContextMenusCoverRowsAndPanelBlank(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "one.md")
	if err := os.WriteFile(file, []byte("# One\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()

	for _, rowName := range []string{"one.md", "Folder"} {
		row, ok := tt.Find(treeDragHandleLabel(rowName))
		if !ok {
			t.Fatalf("tree drag hit area for row %q missing", rowName)
		}
		for _, x := range []float32{row.X + 1, row.X + row.W - 2} {
			tt.RightClickAt(x, row.Y+row.H/2)
			menu := tt.Menu()
			if !containsString(menu, locales.FilesCopyRelativePath()) {
				t.Fatalf("right click at row %q x=%g did not open node menu: %v (row=%+v)", rowName, x, menu, row)
			}
			tt.CloseMenu()
		}
	}

	tree, ok := tt.Find(locales.FilesTreeLabel())
	if !ok {
		t.Fatal("file tree list missing")
	}
	one, _ := tt.Find("one.md")
	blankY := tree.Y + tree.H - 12
	if blankY <= one.Y+one.H || blankY >= tree.Y+tree.H {
		t.Fatalf("no usable blank panel area: tree=%+v row=%+v", tree, one)
	}
	tt.RightClickAt(tree.X+tree.W-12, blankY)
	menu := tt.Menu()
	if !containsString(menu, locales.FilesNewFile()) || !containsString(menu, locales.FilesNewFolderMenu()) {
		t.Fatalf("right click on panel blank did not open safe root menu: %v", menu)
	}
	if containsString(menu, locales.FilesMoveToTrash()) || containsString(menu, locales.FilesRenameEllipsis()) || containsString(menu, locales.FilesCopyItem()) {
		t.Fatalf("root menu exposes node-only action: %v", menu)
	}
}
