package nativeui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestListLevelRootDropPreviewReplacesStaleFolderTarget(t *testing.T) {
	root := filepath.Join("/vault")
	folder := filepath.Join(root, "Folder")
	a := New()
	a.root = root
	a.treeDropTarget = treeDropTarget{path: folder, start: 1, end: 3}
	a.setTreeDragMoveOperation(treeDragMoveToFolder, filepath.Base(folder))
	rows := []fileRow{{path: filepath.Join(root, "note.md")}, {path: folder, dir: true}}

	a.setTreeRootDropTarget(rows)

	if a.treeDropTarget.path != root || a.treeDropTarget.start != 0 || a.treeDropTarget.end != len(rows) {
		t.Fatalf("root fallback preview = %+v, want root range [0,%d)", a.treeDropTarget, len(rows))
	}
	if a.treeDragOperation != treeDragMoveToFolder || a.treeDragTarget != filepath.Base(root) {
		t.Fatalf("root fallback label state = operation %q target %q", a.treeDragOperation, a.treeDragTarget)
	}
}

func TestTreeDragIntoNestedDescendantTargetsContainingDirectory(t *testing.T) {
	root := t.TempDir()
	aPath := filepath.Join(root, "A")
	bPath := filepath.Join(aPath, "B")
	if err := os.MkdirAll(bPath, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.md")
	inside := filepath.Join(bPath, "inside.md")
	for path, content := range map[string]string{source: "source", inside: "inside"} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := New()
	a.setWorkspace(root)
	a.expanded[aPath], a.expanded[bPath] = true, true
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	handle, ok := tt.Find(treeDragHandleLabel("source.md"))
	if !ok {
		t.Fatalf("source drag handle missing; texts=%v", tt.Texts())
	}
	target, ok := tt.Find("inside.md")
	if !ok {
		t.Fatal("nested descendant row missing")
	}
	sx, sy := handle.X+8, handle.Y+14
	tx, ty := target.X+40, target.Y+target.H/2
	tt.Press(sx, sy)
	tt.Move(sx+12, sy)
	tt.Move(tx, ty)
	tt.Frame()
	if a.treeDragTarget != "B" {
		t.Fatalf("nested row target label=%q dragging=%v handle=%+v source=%+v target=%+v rootHoverTarget=%+v texts=%v", a.treeDragTarget, a.treeDragging, handle, handle, target, a.treeDropTarget, tt.Texts())
	}
	tt.Release(tx, ty)
	tt.Frame()
	if _, err := os.Stat(filepath.Join(bPath, "source.md")); err != nil {
		t.Fatalf("source not moved into B: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "source.md")); !os.IsNotExist(err) {
		t.Fatalf("source still at root: %v", err)
	}
}

func TestTreeDragHoverExpandsCollapsedDirectFolderTarget(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.MkdirAll(filepath.Join(folder, "Nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.md")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	handle, _ := tt.Find(treeDragHandleLabel("source.md"))
	target, _ := tt.Find("Folder")
	sx, sy := handle.X+8, handle.Y+14
	tx, ty := target.X+target.W/2, target.Y+target.H/2
	tt.Press(sx, sy)
	tt.Move(sx+12, sy)
	tt.Move(tx, ty)
	tt.Frame()
	tt.Advance(650 * time.Millisecond)
	tt.Frame()
	if !a.expanded[folder] {
		t.Fatalf("directly targeted collapsed directory did not expand; text=%v hover=%q since=%v target=%+v dragging=%v source=%+v targetRow=%+v", tt.Texts(), a.treeDropHoverPath, a.treeDropHoverSince, a.treeDropTarget, a.treeDragging, handle, target)
	}
	if _, ok := tt.Find("Nested"); !ok {
		t.Fatal("expanded folder's child row was not rendered on the follow-up frame")
	}
	if a.treeDropTarget.path != folder {
		t.Fatalf("drop target changed during expansion: %q", a.treeDropTarget.path)
	}
	tt.Move(tx, ty)
	tt.Frame()
	tt.Release(tx, ty)
	tt.Frame()
	if _, err := os.Stat(filepath.Join(folder, "source.md")); err != nil {
		_, rootErr := os.Stat(filepath.Join(root, "source.md"))
		t.Fatalf("drop after auto-expand missed folder: %v root=%v target=%+v op=%q texts=%v", err, rootErr, a.treeDropTarget, a.treeDragOperation, tt.Texts())
	}
}

func TestTreeDragAutoExpandDoesNotDropIfReleasedOutsideTree(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.MkdirAll(filepath.Join(folder, "Nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.md")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	handle, _ := tt.Find(treeDragHandleLabel("source.md"))
	target, _ := tt.Find("Folder")
	sx, sy := handle.X+8, handle.Y+14
	tx, ty := target.X+target.W/2, target.Y+target.H/2
	tt.Press(sx, sy)
	tt.Move(sx+12, sy)
	tt.Move(tx, ty)
	tt.Frame()
	tt.Advance(650 * time.Millisecond)
	tt.Frame()
	if !a.expanded[folder] {
		t.Fatal("folder did not auto-expand")
	}
	tt.Move(800, 350)
	tt.Frame()
	treeBounds, _ := tt.Find(locales.FilesTreeLabel())
	if 800 >= treeBounds.X && 800 < treeBounds.X+treeBounds.W && 350 >= treeBounds.Y && 350 < treeBounds.Y+treeBounds.H {
		t.Fatalf("outside test point unexpectedly inside tree: tree=%+v", treeBounds)
	}
	tt.Release(800, 350)
	tt.Frame()
	if _, err := os.Stat(source); err != nil {
		_, folderErr := os.Stat(filepath.Join(folder, "source.md"))
		t.Fatalf("source moved despite release outside tree: %v folder=%v tree=%+v target=%+v op=%s pointer=%+v", err, folderErr, treeBounds, a.treeDropTarget, a.treeDragOperation, a.treeDragPointer)
	}
	if _, err := os.Stat(filepath.Join(folder, "source.md")); !os.IsNotExist(err) {
		t.Fatalf("stale folder target received outside release: %v", err)
	}
}

func TestTreeDragAutoExpandDoesNotDropOnTreeListBlank(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.MkdirAll(filepath.Join(folder, "Nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.md")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	handle, _ := tt.Find(treeDragHandleLabel("source.md"))
	target, _ := tt.Find("Folder")
	sx, sy := handle.X+8, handle.Y+14
	tx, ty := target.X+target.W/2, target.Y+target.H/2
	tt.Press(sx, sy)
	tt.Move(sx+12, sy)
	tt.Move(tx, ty)
	tt.Frame()
	tt.Advance(650 * time.Millisecond)
	tt.Frame()
	if !a.expanded[folder] {
		t.Fatal("folder did not auto-expand")
	}
	tree, ok := tt.Find(locales.FilesTreeLabel())
	if !ok {
		t.Fatal("file tree list missing")
	}
	blankX, blankY := tree.X+tree.W-10, tree.Y+tree.H-14
	lastRow, ok := tt.Find("Nested")
	if !ok {
		t.Fatal("expanded nested row missing")
	}
	if blankX < tree.X || blankX >= tree.X+tree.W || blankY <= lastRow.Y+lastRow.H || blankY >= tree.Y+tree.H {
		t.Fatalf("release point is not blank list area: list=%+v lastRow=%+v point=(%g,%g)", tree, lastRow, blankX, blankY)
	}
	tt.Move(blankX, blankY)
	tt.Frame()
	tt.Release(blankX, blankY)
	tt.Frame()
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source moved on blank-area release: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "source.md")); !os.IsNotExist(err) {
		t.Fatalf("blank area received stale folder drop: %v", err)
	}
}

func TestTreeDragAutoExpandCanceledPressDoesNotCommit(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.MkdirAll(filepath.Join(folder, "Nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.md")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	handle, _ := tt.Find(treeDragHandleLabel("source.md"))
	target, _ := tt.Find("Folder")
	sx, sy := handle.X+8, handle.Y+14
	tx, ty := target.X+target.W/2, target.Y+target.H/2
	tt.Press(sx, sy)
	tt.Move(sx+12, sy)
	tt.Move(tx, ty)
	tt.Frame()
	tt.Advance(650 * time.Millisecond)
	tt.Frame()
	if !a.expanded[folder] {
		t.Fatal("folder did not auto-expand")
	}
	tt.CancelPress()
	tt.Frame()
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source moved on canceled drag: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "source.md")); !os.IsNotExist(err) {
		t.Fatalf("canceled drag committed stale folder target: %v", err)
	}
}

func TestExternalFileDropOverNestedSubtreeUsesResolvedParent(t *testing.T) {
	root, sourceDir := t.TempDir(), t.TempDir()
	aPath := filepath.Join(root, "A")
	bPath := filepath.Join(aPath, "B")
	if err := os.MkdirAll(bPath, 0o755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(bPath, "inside.md")
	if err := os.WriteFile(inside, []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "external.md")
	if err := os.WriteFile(source, []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(root)
	a.expanded[aPath], a.expanded[bPath] = true, true
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	target, ok := tt.Find("inside.md")
	if !ok {
		t.Fatal("nested destination row missing")
	}
	if !tt.DropFiles(target.X+40, target.Y+target.H/2, source) {
		t.Fatal("tester did not dispatch external drop")
	}
	tt.Frame()
	if got, err := os.ReadFile(filepath.Join(bPath, "external.md")); err != nil || string(got) != "external" {
		_, rootErr := os.Stat(filepath.Join(root, "external.md"))
		t.Fatalf("external file was not imported into B: %q, %v root=%v target=%+v op=%q texts=%v", got, err, rootErr, a.treeDropTarget, a.treeDragOperation, tt.Texts())
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "external" {
		t.Fatalf("external source was modified: %q, %v", got, err)
	}
}

func TestExternalFileDropAfterFolderHoverFallsBackToRootOnListBlank(t *testing.T) {
	root, sourceDir := t.TempDir(), t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(folder, "inside.md")
	if err := os.WriteFile(inside, []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(sourceDir, "first.md")
	second := filepath.Join(sourceDir, "second.md")
	for path, content := range map[string]string{first: "first", second: "second"} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := New()
	a.setWorkspace(root)
	a.expanded[folder] = true
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	folderRow, ok := tt.Find("Folder")
	if !ok {
		t.Fatal("folder destination row missing")
	}
	if !tt.DropFiles(folderRow.X+folderRow.W/2, folderRow.Y+folderRow.H/2, first) {
		t.Fatal("tester did not dispatch first external file drop")
	}
	tt.Frame()
	if got, err := os.ReadFile(filepath.Join(folder, "first.md")); err != nil || string(got) != "first" {
		t.Fatalf("first file was not imported into hovered folder: %q, %v", got, err)
	}
	// The tester dispatches native FileDragOver and FileDrop together. Restore
	// the folder hover as a stale precondition so the blank-area FileDragOver
	// preview must replace it from the list-level fallback path.
	a.treeDropTarget = treeDropTarget{path: folder, start: 0, end: 2}
	a.setTreeDragMoveOperation(treeDragMoveToFolder, filepath.Base(folder))

	tree, ok := tt.Find(locales.FilesTreeLabel())
	if !ok {
		t.Fatal("file tree list missing")
	}
	lastRow, ok := tt.Find("inside.md")
	if !ok {
		t.Fatal("last visible folder child row missing")
	}
	blankX, blankY := tree.X+tree.W-10, tree.Y+tree.H-14
	if blankX < tree.X || blankX >= tree.X+tree.W || blankY <= lastRow.Y+lastRow.H || blankY >= tree.Y+tree.H {
		t.Fatalf("blank drop point is not empty list space: list=%+v lastRow=%+v point=(%g,%g)", tree, lastRow, blankX, blankY)
	}
	if !tt.DropFiles(blankX, blankY, second) {
		t.Fatal("tester did not dispatch external file drop on list blank")
	}
	tt.Frame()
	if got, err := os.ReadFile(filepath.Join(root, "second.md")); err != nil || string(got) != "second" {
		t.Fatalf("blank-area file drop did not use vault root: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(folder, "second.md")); !os.IsNotExist(err) {
		t.Fatalf("blank-area file drop reused stale folder target: %v", err)
	}
}
