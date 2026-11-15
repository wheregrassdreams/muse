package nativeui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"image"
	"image/color"
	"image/png"
	"math"
	"muse/locales"
	"muse/mdedit"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func treeDragHandleLabel(name string) string {
	return locales.TreeDragHandleLabel(name)
}

func clickTreeRow(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("tree item %q missing", label)
	}
	tt.ClickAt(r.X+4, r.Y+r.H/2)
}

func rightClickTreeRow(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("tree item %q missing", label)
	}
	tt.RightClickAt(r.X+4, r.Y+r.H/2)
}

func TestMoveTreeItemIntoFolder(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "note.md")
	destination := filepath.Join(root, "Folder")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root, a.path, a.selectedPath = root, source, source
	a.moveTreeItem(source, destination)
	moved := filepath.Join(destination, "note.md")
	if got, err := os.ReadFile(moved); err != nil || string(got) != "body" {
		t.Fatalf("moved file = %q, %v", got, err)
	}
	if a.path != moved || a.selectedPath != moved {
		t.Fatalf("current paths = %q / %q", a.path, a.selectedPath)
	}
}

func TestMoveTreeFolderRemapsOpenDescendant(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "Old")
	destination := filepath.Join(root, "New")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	open := filepath.Join(source, "note.md")
	if err := os.WriteFile(open, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root, a.path, a.selectedPath = root, open, open
	a.expanded[source] = true
	a.moveTreeItem(source, destination)
	moved := filepath.Join(destination, "Old", "note.md")
	if a.path != moved || a.selectedPath != moved {
		t.Fatalf("open descendant paths = %q / %q", a.path, a.selectedPath)
	}
}

func TestExternalTreeDropCopiesFileWithoutOverwrite(t *testing.T) {
	sourceDir, root := t.TempDir(), t.TempDir()
	source := filepath.Join(sourceDir, "note.md")
	if err := os.WriteFile(source, []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root = root
	a.importDroppedInto([]string{source}, root)
	got, err := os.ReadFile(filepath.Join(root, "note 2.md"))
	if err != nil || string(got) != "external" {
		t.Fatalf("numbered copy = %q, %v", got, err)
	}
	original, err := os.ReadFile(source)
	if err != nil || string(original) != "external" {
		t.Fatalf("source changed = %q, %v", original, err)
	}
}

func TestExternalTreeDropSkipsTopLevelSymlink(t *testing.T) {
	sourceDir, root := t.TempDir(), t.TempDir()
	target := filepath.Join(sourceDir, "real.md")
	if err := os.WriteFile(target, []byte("real file"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(sourceDir, "linked.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	a := New()
	a.root = root
	a.importDroppedInto([]string{link}, root)
	if _, err := os.Lstat(filepath.Join(root, filepath.Base(link))); !os.IsNotExist(err) {
		t.Fatalf("top-level symlink was imported: %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "real file" {
		t.Fatalf("symlink target changed: %q, %v", got, err)
	}
}

func TestMoveTreeItemToCurrentParentIsNoOp(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note with spaces.md")
	if err := os.WriteFile(path, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root, a.path, a.selectedPath = root, path, path
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeOrder := strings.Join(a.children(root), "\n")
	a.moveTreeItem(path, root)
	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if a.path != path || a.selectedPath != path || beforeInfo.ModTime() != afterInfo.ModTime() || beforeOrder != strings.Join(a.children(root), "\n") {
		t.Fatalf("drop to current parent changed path/order/metadata: path=%q selected=%q", a.path, a.selectedPath)
	}
}

func TestPlainTreeClickDoesNotStartDragProjection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("# note"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	before, ok := tt.Find("note.md")
	if !ok {
		t.Fatal("tree row missing")
	}
	if a.treeDragging {
		t.Fatal("tree drag UI appeared before input")
	}
	tt.ClickAt(before.X+4, before.Y+10)
	tt.Frame()
	after, ok := tt.Find("note.md")
	if !ok || before.H != after.H || before.Y != after.Y {
		t.Fatalf("row geometry changed on click: before=%+v after=%+v", before, after)
	}
	if a.treeDragging {
		t.Fatal("plain click started drag UI")
	}
	if _, found := tt.Find("tree-drag-projection"); found {
		t.Fatal("plain click displayed drag projection")
	}
}

func TestTreeModifierSelectionDoesNotOpenOrToggleRows(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	if err := tt.ClickWith(0, treeDragHandleLabel("a.md")); err != nil {
		t.Fatal(err)
	}
	if a.selectedPath != filepath.Join(root, "a.md") || !a.treeSelection[filepath.Join(root, "a.md")] {
		t.Fatalf("plain click did not select/open one file: selected=%v open=%q", a.treeSelection, a.selectedPath)
	}
	if err := tt.ClickWith(ui.Cmd, treeDragHandleLabel("b.md")); err != nil {
		t.Fatal(err)
	}
	if len(a.treeSelection) != 2 || !a.treeSelection[filepath.Join(root, "a.md")] || !a.treeSelection[filepath.Join(root, "b.md")] {
		t.Fatalf("Cmd/Ctrl click selection = %v", a.treeSelection)
	}
	if a.selectedPath != filepath.Join(root, "a.md") {
		t.Fatalf("modifier click opened another file: selectedPath=%q", a.selectedPath)
	}
	if err := tt.ClickWith(ui.Cmd, treeDragHandleLabel("b.md")); err != nil {
		t.Fatal(err)
	}
	if len(a.treeSelection) != 1 || !a.treeSelection[filepath.Join(root, "a.md")] {
		t.Fatalf("Cmd/Ctrl click did not toggle the row off: %v", a.treeSelection)
	}
	if err := tt.ClickWith(ui.Cmd, treeDragHandleLabel("b.md")); err != nil {
		t.Fatal(err)
	}
	if err := tt.ClickWith(ui.Shift, treeDragHandleLabel("c.md")); err != nil {
		t.Fatal(err)
	}
	if len(a.treeSelection) != 3 || !a.treeSelection[filepath.Join(root, "a.md")] || !a.treeSelection[filepath.Join(root, "b.md")] || !a.treeSelection[filepath.Join(root, "c.md")] {
		t.Fatalf("Shift range selection = %v", a.treeSelection)
	}
	if a.treeSelectionAnchor != filepath.Join(root, "a.md") || a.selectedPath != filepath.Join(root, "a.md") {
		t.Fatalf("Shift click changed anchor/open file: anchor=%q selected=%q", a.treeSelectionAnchor, a.selectedPath)
	}
	if err := tt.ClickWith(ui.Cmd, treeDragHandleLabel("Folder")); err != nil {
		t.Fatal(err)
	}
	if a.expanded[folder] || !a.treeSelection[folder] {
		t.Fatalf("modifier click toggled folder or missed selection: expanded=%v selection=%v", a.expanded[folder], a.treeSelection)
	}
	clickTreeRow(t, tt, "Folder")
	if !a.expanded[folder] {
		t.Fatal("plain folder click did not toggle expansion")
	}
}

func TestTreeRightClickPreservesSelectionOnlyForSelectedRow(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	clickTreeRow(t, tt, "a.md")
	if err := tt.ClickWith(ui.Cmd, treeDragHandleLabel("b.md")); err != nil {
		t.Fatal(err)
	}
	rightClickTreeRow(t, tt, "b.md")
	if len(a.treeSelection) != 2 || !a.treeSelection[filepath.Join(root, "a.md")] || !a.treeSelection[filepath.Join(root, "b.md")] {
		t.Fatalf("right-click selected row discarded the multi-selection: %v", a.treeSelection)
	}
	rightClickTreeRow(t, tt, "c.md")
	if len(a.treeSelection) != 1 || !a.treeSelection[filepath.Join(root, "c.md")] {
		t.Fatalf("right-click unselected row did not establish its own context: %v", a.treeSelection)
	}
	if a.selectedPath != filepath.Join(root, "a.md") {
		t.Fatalf("right-click opened another file: %q", a.selectedPath)
	}
}

func TestTreeDragMovesCurrentMultiSelection(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	clickTreeRow(t, tt, "a.md")
	if err := tt.ClickWith(ui.Cmd, treeDragHandleLabel("b.md")); err != nil {
		t.Fatal(err)
	}
	handle, ok := tt.Find(treeDragHandleLabel("b.md"))
	if !ok {
		t.Fatal("multi-selection drag handle missing")
	}
	target, ok := tt.Find("Folder")
	if !ok {
		t.Fatal("folder drop target missing")
	}
	tt.Press(handle.X+4, handle.Y+14)
	tt.Frame()
	tt.Move(handle.X+14, handle.Y+14)
	tt.Move(target.X+target.W/2, target.Y+target.H/2)
	tt.Frame()
	tt.Release(target.X+target.W/2, target.Y+target.H/2)
	tt.Frame()
	for _, name := range []string{"a.md", "b.md"} {
		if _, err := os.Stat(filepath.Join(folder, name)); err != nil {
			t.Fatalf("selected item %s was not moved: %v (selection=%v dragSelection=%v dragged=%v operation=%q)", name, err, a.treeSelection, a.treeDragSelection, a.treeDragging, a.treeDragOperation)
		}
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("selected item %s remained at its source: %v", name, err)
		}
	}
	for _, name := range []string{"a.md", "b.md"} {
		if !a.treeSelection[filepath.Join(folder, name)] {
			t.Fatalf("selection was not remapped to moved path %q: %v", filepath.Join(folder, name), a.treeSelection)
		}
	}
	if a.treeSelectionAnchor != filepath.Join(folder, "a.md") {
		t.Fatalf("selection anchor was not remapped: %q", a.treeSelectionAnchor)
	}
}

func TestTreeDragUnmodifiedPressOnUnselectedRowCollapsesGroup(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	clickTreeRow(t, tt, "a.md")
	handle, ok := tt.Find(treeDragHandleLabel("b.md"))
	if !ok {
		t.Fatal("unselected drag handle missing")
	}
	x, y := handle.X+12, handle.Y+14
	tt.Press(x, y)
	tt.Frame()
	if len(a.treeSelection) != 1 || !a.treeSelection[filepath.Join(root, "a.md")] {
		t.Fatalf("selection changed before threshold: %v", a.treeSelection)
	}
	tt.Move(x+10, y)
	tt.Frame()
	if len(a.treeDragSelection) != 1 || a.treeDragSelection[0] != filepath.Join(root, "b.md") {
		t.Fatalf("dragging an unselected row should collapse to itself: selection=%v frozen=%v", a.treeSelection, a.treeDragSelection)
	}
	target, ok := tt.Find("Folder")
	if !ok {
		t.Fatal("folder drop target missing")
	}
	tt.Move(target.X+target.W/2, target.Y+target.H/2)
	tt.Frame()
	tt.Release(target.X+target.W/2, target.Y+target.H/2)
	tt.Frame()
	if _, err := os.Stat(filepath.Join(folder, "b.md")); err != nil {
		t.Fatalf("drag did not move the pressed row: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "a.md")); !os.IsNotExist(err) {
		t.Fatalf("drag unexpectedly included prior selection: %v", err)
	}
}

func TestTreeDragTrashesCurrentMultiSelection(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "a.md"), filepath.Join(root, "b.md")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte(filepath.Base(path)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := New()
	a.setWorkspace(root)
	trashed := make(chan string, len(paths))
	oldTrash, oldDialog := trashItem, messageDialog
	trashItem = func(path string) error { trashed <- path; return nil }
	messageDialog = func(mygo.MessageOptions) (mygo.MessageResult, error) { return mygo.MessageResult{Button: 0}, nil }
	defer func() { trashItem, messageDialog = oldTrash, oldDialog }()
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	clickTreeRow(t, tt, "a.md")
	if err := tt.ClickWith(ui.Cmd, treeDragHandleLabel("b.md")); err != nil {
		t.Fatal(err)
	}
	handle, ok := tt.Find(treeDragHandleLabel("b.md"))
	if !ok {
		t.Fatal("multi-selection drag handle missing")
	}
	tt.Press(handle.X+4, handle.Y+14)
	tt.Move(handle.X+14, handle.Y+14)
	tt.Move(500, 680)
	tt.Frame()
	trashText, ok := tt.Find(locales.TreeTrashHint())
	if !ok {
		t.Fatal("trash drop target missing during multi-drag")
	}
	tt.Move(trashText.X+trashText.W/2, trashText.Y+trashText.H/2)
	tt.Frame()
	tt.Release(trashText.X+trashText.W/2, trashText.Y+trashText.H/2)
	tt.Frame()
	seen := map[string]bool{}
	for range paths {
		select {
		case path := <-trashed:
			seen[path] = true
		case <-time.After(time.Second):
			t.Fatalf("multi-selection trash did not process both items: %v", seen)
		}
	}
	for _, path := range paths {
		if !seen[path] {
			t.Fatalf("selected item %q was not sent to recoverable trash: %v", path, seen)
		}
	}
}

func TestTreeDragUsesOneProjectionAfterThresholdAndKeepsRowFixed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("# note"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(root)
	a.loadDoc("# Draft\n\n")
	original := a.ed.Source()
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	before, ok := tt.Find("note.md")
	if !ok {
		t.Fatal("tree row missing")
	}
	handle, ok := tt.Find(treeDragHandleLabel("note.md"))
	if !ok {
		t.Fatal("tree drag handle missing")
	}
	startX, startY := handle.X+4, handle.Y+14
	initialProjection, ok := tt.Find(locales.TreeDragProjectionLabel("note.md"))
	if !ok {
		t.Fatal("projection card missing")
	}
	tt.Press(startX, startY)
	tt.Move(startX+10, startY)
	tt.Frame()
	if !a.treeDragging {
		t.Fatal("crossing drag threshold did not start the tree drag")
	}
	tt.Advance(150 * time.Millisecond)
	tt.Frame()
	tt.Move(600, 360)
	tt.Frame()
	tt.Frame()
	tt.Frame()
	tt.Advance(200 * time.Millisecond)
	tt.Frame()
	finalProjection, ok := tt.Find(locales.TreeDragProjectionLabel("note.md"))
	initialCenterX := initialProjection.X + initialProjection.W/2
	finalCenterX := finalProjection.X + finalProjection.W/2
	if !ok || finalProjection.W >= initialProjection.W || finalProjection.H <= initialProjection.H || math.Abs(float64(finalCenterX-initialCenterX)) > 1 {
		t.Fatalf("projection did not compact and grow: initial=%+v final=%+v started=%v dragging=%v", initialProjection, finalProjection, a.treeDragStarted, a.treeDragging)
	}
	after, ok := tt.Find("note.md")
	if !ok || before.X != after.X || before.Y != after.Y || before.W != after.W || before.H != after.H {
		t.Fatalf("source row geometry changed during drag: before=%+v after=%+v", before, after)
	}
	texts := strings.Join(tt.Texts(), "\n")
	if a.treeDragOperation != treeDragInsertIntoEditor {
		t.Fatalf("editor target operation = %q", a.treeDragOperation)
	}
	if !strings.Contains(texts, locales.TreeDragProjectionOperationLabel("note.md")) {
		t.Fatalf("projection operation is missing from the dragged item: operation=%q texts=%q", a.treeDragOperation, texts)
	}
	runtimeShot(t, tt, "tree-drag-projection")
	tt.Release(600, 360)
	tt.Frame()
	if a.treeDragging {
		t.Fatal("tree drag UI remained after release")
	}
	if !strings.Contains(a.ed.Source(), "[[note]]") {
		t.Fatalf("editor drop did not insert wiki link: %q", a.ed.Source())
	}
	if !a.ed.Undo() || a.ed.Source() != original {
		t.Fatalf("undo did not revert link insertion: %q", a.ed.Source())
	}
}

func TestTreeDragDropsFileIntoFolder(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "note.md")
	if err := os.WriteFile(source, []byte("# note"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	handle, ok := tt.Find(treeDragHandleLabel("note.md"))
	if !ok {
		t.Fatal("file drag handle missing")
	}
	target, ok := tt.Find("Folder")
	if !ok {
		t.Fatal("folder row missing")
	}
	tt.Press(handle.X+4, handle.Y+14)
	tt.Move(handle.X+14, handle.Y+14)
	tt.Move(target.X+target.W/2, target.Y+target.H/2)
	tt.Frame()
	if !a.treeDragging {
		t.Fatalf("drag source did not remain active at target: handle=%+v target=%+v", handle, target)
	}
	if a.treeDragOperation != treeDragMoveToFolder || a.treeDragTarget != "Folder" {
		t.Fatalf("folder target operation = %q target=%q handle=%+v target=%+v", a.treeDragOperation, a.treeDragTarget, handle, target)
	}
	tt.Release(target.X+target.W/2, target.Y+target.H/2)
	tt.Frame()
	if _, err := os.Stat(filepath.Join(folder, "note.md")); err != nil {
		t.Fatalf("drop did not move file: %v", err)
	}
}

func TestTreeDragDropsToWideRootBlankArea(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(folder, "note.md")
	if err := os.WriteFile(source, []byte("# note"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(root)
	a.expanded[folder] = true
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	handle, ok := tt.Find(treeDragHandleLabel("note.md"))
	if !ok {
		t.Fatal("file drag handle missing")
	}
	tt.Press(handle.X+4, handle.Y+14)
	tt.Move(handle.X+14, handle.Y+14)
	tt.Move(220, 420)
	tt.Frame()
	if a.treeDragOperation != treeDragMoveToFolder || a.treeDragTarget != filepath.Base(root) {
		t.Fatalf("root drop target = operation %q target %q", a.treeDragOperation, a.treeDragTarget)
	}
	tt.Release(220, 420)
	tt.Frame()
	if _, err := os.Stat(filepath.Join(root, "note.md")); err != nil {
		t.Fatalf("root blank area did not accept drop: %v", err)
	}
}

func TestTreeDragDropsIntoTrashUsesRecoverableTrashAction(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "note.md")
	if err := os.WriteFile(source, []byte("# note"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(root)
	trashed := make(chan string, 1)
	oldTrash, oldDialog := trashItem, messageDialog
	trashItem = func(path string) error { trashed <- path; return nil }
	messageDialog = func(mygo.MessageOptions) (mygo.MessageResult, error) { return mygo.MessageResult{Button: 0}, nil }
	defer func() { trashItem, messageDialog = oldTrash, oldDialog }()
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	handle, ok := tt.Find(treeDragHandleLabel("note.md"))
	if !ok {
		t.Fatal("file drag handle missing")
	}
	tt.Press(handle.X+4, handle.Y+14)
	tt.Move(handle.X+14, handle.Y+14)
	tt.Move(500, 680)
	tt.Frame()
	trashTarget, ok := tt.Find(locales.TreeTrashDropTarget())
	if !ok {
		t.Fatal("bottom trash target missing during drag")
	}
	footer, ok := tt.Find(locales.SidebarFixedFooter())
	if !ok {
		t.Fatal("fixed sidebar footer missing during drag")
	}
	if trashTarget.X < 9 || trashTarget.X+trashTarget.W > a.panelW-9 {
		t.Fatalf("trash target must have sidebar side margins: bounds=%+v panelW=%.0f", trashTarget, a.panelW)
	}
	if trashTarget.Y+trashTarget.H > footer.Y-9 {
		t.Fatalf("trash target must end above the fixed footer with a 9 DIP gap: target=%+v footer=%+v", trashTarget, footer)
	}
	if footer.Y-(trashTarget.Y+trashTarget.H) > 16 {
		t.Fatalf("trash target should cover the lower tree area just above the footer: target=%+v footer=%+v", trashTarget, footer)
	}
	runtimeShot(t, tt, "tree-trash-above-footer-overlay")
	tt.Move(trashTarget.X+trashTarget.W/2, trashTarget.Y+trashTarget.H/2)
	tt.Frame()
	runtimeShot(t, tt, "tree-trash-above-footer-overlay-hover")
	tt.Release(trashTarget.X+trashTarget.W/2, trashTarget.Y+trashTarget.H/2)
	tt.Frame()
	select {
	case got := <-trashed:
		if got != source {
			t.Fatalf("trash path = %q, want %q", got, source)
		}
	case <-time.After(time.Second):
		t.Fatal("trash drop did not invoke recoverable trash action")
	}
}

func TestTreeDragHandsOffToNativeWindowDragWhenPointerLeaves(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note with spaces.md")
	if err := os.WriteFile(path, []byte("# note"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(root)
	var got transfer.Data
	var options mygo.DragOptions
	oldStart := handoffTreeNativeDrag
	handoffTreeNativeDrag = func(_ *ui.Context, data transfer.Data, opts mygo.DragOptions) error {
		got, options = data, opts
		return nil
	}
	defer func() { handoffTreeNativeDrag = oldStart }()
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	handle, ok := tt.Find(treeDragHandleLabel("note with spaces.md"))
	if !ok {
		t.Fatal("file drag handle missing")
	}
	tt.Press(handle.X+4, handle.Y+14)
	tt.Move(handle.X+14, handle.Y+14)
	tt.Frame()
	if !a.treeDragging {
		t.Fatal("in-window drag did not start")
	}
	tt.Move(-20, -20)
	tt.Frame()
	if !a.treeNativeHandoff {
		t.Fatal("leaving the window did not start native drag handoff")
	}
	fileURL, err := got.Read(transfer.FileList)
	if err != nil || !strings.Contains(string(fileURL), "file://") || !strings.Contains(string(fileURL), "%20") {
		t.Fatalf("native transfer lacks file URL: %q %v", fileURL, err)
	}
	b, err := got.Read(transfer.Format("application/x-muse-vault-item+json"))
	if err != nil || !strings.Contains(string(b), `"path":"`+path+`"`) {
		t.Fatalf("native transfer lacks Muse item metadata: %s %v", b, err)
	}
	if options.Operations != transfer.Copy|transfer.Move {
		t.Fatalf("native operations = %v", options.Operations)
	}
	if options.Done == nil {
		t.Fatal("native handoff did not register completion")
	}
	tt.Move(handle.X+4, handle.Y+14)
	tt.Frame()
	if !a.treeDragging || a.treeDragPointer.X <= 0 {
		t.Fatal("re-entering Muse did not restore the in-window projection state")
	}
	options.Done(transfer.Result{Canceled: true})
	if a.treeNativeHandoff {
		t.Fatal("native handoff state remained after completion")
	}
	if len(a.treeDragSelection) != 0 {
		t.Fatalf("native handoff retained stale drag selection: %v", a.treeDragSelection)
	}
}

func TestTreeNativeHandoffCompletionAllowsNextClick(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"source.md", "next.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	source, ok := tt.Find(treeDragHandleLabel("source.md"))
	if !ok {
		t.Fatal("source row missing")
	}
	tt.Press(source.X+4, source.Y+14)
	tt.Move(source.X+14, source.Y+14)
	tt.Frame()
	tt.Move(-20, -20)
	tt.Frame()
	if !a.treeNativeHandoff {
		t.Fatal("pointer exit did not promote the active drag")
	}
	// Escape cancels the headless native session and runs the same completion
	// wrapper as an OS drop, including clearing the original pressed row.
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.treeNativeHandoff || a.treeDragging {
		t.Fatalf("native completion retained app drag state: handoff=%v dragging=%v", a.treeNativeHandoff, a.treeDragging)
	}
	next, ok := tt.Find(treeDragHandleLabel("next.md"))
	if !ok {
		t.Fatal("next row missing")
	}
	tt.ClickAt(next.X+next.W/2, next.Y+next.H/2)
	if a.selectedPath != filepath.Join(root, "next.md") {
		t.Fatalf("next click was swallowed by stale press: selected=%q", a.selectedPath)
	}
}

func TestCrossWindowNativeMoveBackMovesOriginalOnlyOnce(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(root, "note with spaces.md")
	if err := os.WriteFile(original, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	sender := New()
	sender.root = root
	sender.children(root) // Seed the sender's cache before the other window moves it.
	data, err := treeItemTransferData(mdedit.VaultItemDrag{Path: original, Name: filepath.Base(original)})
	if err != nil {
		t.Fatal(err)
	}
	opts := sender.treeNativeDragOptions(original)

	// A drag into another Muse window in this vault is committed by the
	// receiver. Source Done reports the native result, but must not move or
	// remove the source a second time.
	if opts.Done == nil {
		t.Fatal("native source did not register Done")
	}
	firstReceiver := New()
	firstReceiver.root = root
	// MyGo negotiates Copy when Suggested is None. Muse interprets that
	// unmodified same-vault native drag as its default Move.
	firstReceiver.treeNativeDropSuggested = transfer.None
	firstReceiver.applyTreeNativeDrop(transfer.Drop{Data: data, Operation: transfer.Copy}, folder)
	firstMoved := filepath.Join(folder, filepath.Base(original))
	opts.Done(transfer.Result{Operation: transfer.Copy})
	if got := sender.children(root); len(got) != 1 || got[0] != folder {
		t.Fatalf("sender root cache after native move = %v, want only %q", got, folder)
	}
	if got := sender.children(folder); len(got) != 1 || got[0] != firstMoved {
		t.Fatalf("sender destination cache after native move = %v, want %q", got, firstMoved)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("source remained after first receiver move: %v", err)
	}
	if got, err := os.ReadFile(firstMoved); err != nil || string(got) != "body" {
		t.Fatalf("first receiver target = %q, %v", got, err)
	}

	// Drag the same item back to the other window's root and finish the
	// source-side native callback. Exactly one copy must exist afterward.
	sender.treeNativeHandoff = false
	sender.children(folder) // Cache the item's current location before moving back.
	backData, err := treeItemTransferData(mdedit.VaultItemDrag{Path: firstMoved, Name: filepath.Base(firstMoved)})
	if err != nil {
		t.Fatal(err)
	}
	backOpts := sender.treeNativeDragOptions(firstMoved)
	secondReceiver := New()
	secondReceiver.root = root
	secondReceiver.treeNativeDropSuggested = transfer.None
	secondReceiver.applyTreeNativeDrop(transfer.Drop{Data: backData, Operation: transfer.Copy}, root)
	backOpts.Done(transfer.Result{Operation: transfer.Copy})
	if got := sender.children(folder); len(got) != 0 {
		t.Fatalf("sender old-folder cache after move back = %v, want empty", got)
	}
	if got := sender.children(root); len(got) != 2 || got[0] != folder || got[1] != original {
		t.Fatalf("sender root cache after move back = %v, want folder and returned file", got)
	}
	if _, err := os.Stat(firstMoved); !os.IsNotExist(err) {
		t.Fatalf("old location remained after move back: %v", err)
	}
	returned := filepath.Join(root, filepath.Base(original))
	if got, err := os.ReadFile(returned); err != nil || string(got) != "body" {
		t.Fatalf("returned file = %q, %v", got, err)
	}
	count := 0
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == filepath.Base(original) {
			count++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("matching file count after move back = %d, want one", count)
	}
}

func TestCrossWindowNativeCopyKeepsSource(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "note.md")
	if err := os.WriteFile(source, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := treeItemTransferData(mdedit.VaultItemDrag{Path: source, Name: "note.md"})
	if err != nil {
		t.Fatal(err)
	}
	receiver := New()
	receiver.root = root
	receiver.treeNativeDropSuggested = transfer.Copy
	receiver.applyTreeNativeDrop(transfer.Drop{Data: data, Operation: transfer.Copy}, folder)
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("copy removed source: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(folder, "note.md")); err != nil || string(got) != "body" {
		t.Fatalf("copy target = %q, %v", got, err)
	}
}

func TestCrossWindowNativeDefaultMoveMovesSelectedItems(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(root, "a.md"), filepath.Join(root, "b.md")}
	items := make([]mdedit.VaultItemDrag, 0, len(paths))
	for _, path := range paths {
		if err := os.WriteFile(path, []byte(filepath.Base(path)), 0o644); err != nil {
			t.Fatal(err)
		}
		items = append(items, mdedit.VaultItemDrag{Path: path, Name: filepath.Base(path)})
	}
	data, err := treeItemsTransferData(items)
	if err != nil {
		t.Fatal(err)
	}
	receiver := New()
	receiver.root = root
	receiver.treeNativeDropSuggested = transfer.None
	receiver.applyTreeNativeDrop(transfer.Drop{Data: data, Operation: transfer.Copy}, folder)
	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("default Move left source %q: %v", path, err)
		}
		if _, err := os.Stat(filepath.Join(folder, filepath.Base(path))); err != nil {
			t.Fatalf("default Move omitted selected item %q: %v", path, err)
		}
	}
}

func TestTreeImageHoverPopupSitsOutsideClippedSidebar(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "photo.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 32, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: 80, G: 140, B: 220, A: 255})
		}
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	row, ok := tt.Find("photo.png")
	if !ok {
		t.Fatal("image row missing")
	}
	tt.Move(row.X+row.W/2, row.Y+row.H/2)
	tt.Frame()
	popup, ok := tt.Find("图片预览")
	if !ok {
		t.Fatalf("image hover popup missing; path=%q rect=%+v texts=%q", a.treePreviewPath, row, tt.Texts())
	}
	if popup.X < row.X+row.W || popup.X-row.X-row.W > 12 || popup.Y != row.Y || popup.X+popup.W > 1000 || popup.Y+popup.H > 720 {
		t.Fatalf("popup is clipped/offscreen: %+v panelWidth=%.0f", popup, a.panelW)
	}
	runtimeShot(t, tt, "tree-image-hover-popup")
}

func TestDraggingTreeImageShowsThumbnailAndHidesHoverPreview(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "photo.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 48, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 48; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 5), G: uint8(y * 8), B: 180, A: 255})
		}
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	a := New()
	a.setWorkspace(root)
	a.loadDoc("# Draft\n\n")
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	row, ok := tt.Find("photo.png")
	if !ok {
		t.Fatal("image row missing")
	}
	tt.Move(row.X+row.W/2, row.Y+row.H/2)
	tt.Frame()
	if a.treePreviewPath != path {
		t.Fatalf("hover preview was not established before drag: %q", a.treePreviewPath)
	}
	handle, ok := tt.Find(treeDragHandleLabel("photo.png"))
	if !ok {
		t.Fatal("image drag handle missing")
	}
	tt.Press(handle.X+4, handle.Y+14)
	tt.Move(handle.X+14, handle.Y+14)
	tt.Frame()
	if !a.treeDragging {
		t.Fatal("crossing drag threshold did not start the image drag")
	}
	if a.treePreviewPath != "" {
		t.Fatalf("tree image preview remained during drag: %q", a.treePreviewPath)
	}
	if _, ok := tt.Find("拖动图片缩略图"); !ok {
		t.Fatalf("image thumbnail missing from drag projection: %q", strings.Join(tt.Texts(), "\n"))
	}
	if _, ok := tt.Find("图片预览"); ok {
		t.Fatal("hover preview popup remained visible during drag")
	}
	runtimeShot(t, tt, "tree-image-drag-thumbnail")
	tt.Move(600, 360)
	tt.Frame()
	tt.Release(600, 360)
	tt.Frame()
	if !strings.Contains(a.ed.Source(), "![[photo.png]]") {
		t.Fatalf("tree image drop did not insert embed syntax: %q", a.ed.Source())
	}
}
