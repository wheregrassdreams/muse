package nativeui

import (
	"fmt"
	"math"
	"muse/locales"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestTreeMultiSelectionProjectionShowsCountAndStackedIcon(t *testing.T) {
	cases := []struct {
		name  string
		mods  ui.Modifiers
		press string
		want  []string
		shot  string
	}{
		{name: "cmd-toggle-press-drag", mods: ui.Cmd, press: "b.md", want: []string{"a.md", "b.md"}, shot: "tree-multiselect-cmd-drag"},
		{name: "shift-range-press-drag", mods: ui.Shift, press: "c.md", want: []string{"a.md", "b.md", "c.md"}, shot: "tree-multiselect-shift-drag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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
			a := newFileTreeTestApp(t, root)
			a.setWorkspace(root)
			tt := ui.NewTester(a.View, 1000, 720)
			tt.SetDark(true)
			tt.Frame()
			clickTreeRow(t, tt, "a.md")
			handle, ok := tt.Find(treeDragHandleLabel(tc.press))
			if !ok {
				t.Fatal("drag handle missing")
			}
			pressX, pressY := handle.X+12, handle.Y+14
			tt.PressWithModifiers(tc.mods, 0, pressX, pressY)
			tt.Frame()
			if len(a.treeSelection) != 1 || !a.treeSelection[filepath.Join(root, "a.md")] {
				t.Fatalf("modifier press changed selection before drag threshold: %v", a.treeSelection)
			}
			if a.treeDragging {
				t.Fatal("press alone started drag before threshold")
			}
			tt.Move(pressX+1, pressY)
			tt.Frame()
			if a.treeDragging {
				t.Fatal("one DIP motion crossed the drag threshold")
			}
			if len(a.treeSelection) != 1 || !a.treeSelection[filepath.Join(root, "a.md")] {
				t.Fatalf("sub-threshold motion changed selection: %v", a.treeSelection)
			}
			tt.Move(pressX+12, pressY)
			tt.Frame()
			if !a.treeDragging {
				t.Fatal("modifier press followed by threshold motion did not start drag")
			}
			if len(a.treeDragSelection) != len(tc.want) {
				t.Fatalf("frozen drag selection = %v; want %v", a.treeDragSelection, tc.want)
			}
			for _, name := range tc.want {
				if !a.treeSelection[filepath.Join(root, name)] {
					t.Fatalf("drag threshold selection missed %s: %v", name, a.treeSelection)
				}
			}
			countLabel := locales.FilesSelectedFileCount(len(tc.want))
			assertProjectionCount := func(stage string) {
				t.Helper()
				_, cardOK := tt.Find(locales.TreeDragProjectionLabel(tc.press))
				count, countOK := tt.Find(countLabel)
				allText := strings.Join(tt.Texts(), "\n")
				if !cardOK || !countOK || strings.Count(allText, countLabel) != 1 {
					t.Fatalf("%s projection lost/forked selection count: count=%+v texts=%q selection=%v frozen=%v", stage, count, allText, a.treeSelection, a.treeDragSelection)
				}
			}
			assertProjectionCount("first threshold")
			target, ok := tt.Find("Folder")
			if !ok {
				t.Fatal("folder target missing")
			}
			tt.Move(target.X+target.W/2, target.Y+target.H/2)
			tt.Frame()
			assertProjectionCount("folder hover")
			tt.Advance(150 * time.Millisecond)
			tt.Frame()
			assertProjectionCount("settled folder hover")
			_, projectionOK := tt.Find(locales.TreeDragProjectionLabel(tc.press))
			_, opOK := tt.Find(locales.TreeDragProjectionOperationLabel(tc.press))
			if a.treeDragOperation != treeDragMoveToFolder || a.treeDragTarget != "Folder" || !projectionOK || !opOK {
				t.Fatalf("target operation not represented on the dragged item projection: operation=%q texts=%v", a.treeDragOperation, tt.Texts())
			}
			runtimeShot(t, tt, tc.shot)
			tt.Release(target.X+target.W/2, target.Y+target.H/2)
			tt.Frame()
			for _, name := range tc.want {
				if _, err := os.Stat(filepath.Join(folder, name)); err != nil {
					t.Fatalf("drag did not move selected %s: %v", name, err)
				}
			}
		})
	}
}

func TestTreeProjectionNamesMixedSelectionAsProjects(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "note.md")
	if err := os.WriteFile(file, []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}
	if count, filesOnly := treeProjectionSelectionLabel([]string{file, folder}, func(path string) bool { return path == folder }); count != 2 || filesOnly {
		t.Fatalf("mixed selection = %d items, filesOnly=%v; want 2 items and filesOnly=false", count, filesOnly)
	}
}

func TestTreeTrashHoverKeepsCompactTwoLineProjection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newFileTreeTestApp(t, root)
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.SetDark(true)
	tt.Frame()
	handle, ok := tt.Find(treeDragHandleLabel("note.md"))
	if !ok {
		t.Fatal("file drag handle missing")
	}
	tt.Press(handle.X+4, handle.Y+14)
	tt.Move(handle.X+14, handle.Y+14)
	tt.Move(500, 680)
	tt.Frame()
	target, ok := tt.Find(locales.TreeTrashDropTarget())
	if !ok {
		t.Fatal("trash target missing during drag")
	}
	tt.Move(target.X+target.W/2, target.Y+target.H/2)
	tt.Frame()
	// Exercise the delayed frames used by the real pointer-driven projection.
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	tt.Frame()
	if a.treeDragOperation != treeDragMoveToTrash {
		t.Fatalf("trash target operation = %q", a.treeDragOperation)
	}
	target, ok = tt.Find(locales.TreeTrashDropTarget())
	if !ok {
		t.Fatal("trash target disappeared on hover")
	}
	column, columnOK := tt.Find(locales.TreeTrashHintColumn())
	_, hintVisible := tt.Find(locales.TreeTrashHint())
	icon, iconVisible := tt.Find(locales.TreeTrashHoverIcon())
	centerDelta := (column.X + column.W/2) - (target.X + target.W/2)
	if centerDelta < 0 {
		centerDelta = -centerDelta
	}
	iconCenterDelta := (icon.X + icon.W/2) - (target.X + target.W/2)
	if iconCenterDelta < 0 {
		iconCenterDelta = -iconCenterDelta
	}
	if !columnOK || hintVisible || !iconVisible || centerDelta > 1 || iconCenterDelta > 1 || column.X < target.X || column.X+column.W > target.X+target.W || icon.X < column.X || icon.X+icon.W > column.X+column.W {
		t.Fatalf("trash hover icon must stay centered while the overlay label is hidden: target=%+v column=%+v icon=%+v hintVisible=%v", target, column, icon, hintVisible)
	}
	card, ok := tt.Find(locales.TreeDragProjectionLabel("note.md"))
	if !ok || card.W > 240 || card.H < 48 || card.H > 54 || card.Y+card.H > target.Y {
		t.Fatalf("trash projection is not the compact two-line card: %+v", card)
	}
	op, ok := tt.Find(locales.TreeDragProjectionOperationLabel("note.md"))
	if !ok || op.X < card.X || op.X+op.W > card.X+card.W || op.Y < card.Y || op.Y+op.H > card.Y+card.H {
		t.Fatalf("trash operation is absent or outside projection: op=%+v card=%+v text=%v", op, card, tt.Texts())
	}
	if !tt.HasText(locales.TreeDragProjectionOperationLabel("note.md")) {
		t.Fatalf("projection lost the trash operation: %v", tt.Texts())
	}
	runtimeShot(t, tt, "tree-trash-projection-hover")
	tt.Release(700, 300)
	tt.Frame()
}

func TestTreeDragProjectionSurvivesCollapsedFolderExpansion(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Folder")
	if err := os.MkdirAll(filepath.Join(folder, "Nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.md")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newFileTreeTestApp(t, root)
	a.setWorkspace(root)
	tt := ui.NewTester(a.View, 1000, 720)
	tt.Frame()
	handle, ok := tt.Find(treeDragHandleLabel("source.md"))
	if !ok {
		t.Fatal("source drag handle missing")
	}
	target, ok := tt.Find("Folder")
	if !ok {
		t.Fatal("collapsed folder row missing")
	}
	startX, startY := handle.X+8, handle.Y+14
	targetX, targetY := target.X+target.W/2, target.Y+target.H/2
	tt.Press(startX, startY)
	tt.Move(startX+12, startY)
	tt.Move(targetX, targetY)
	tt.Frame()
	if !a.treeDragging || a.treeDragItem.Path != source {
		t.Fatalf("drag did not start from source: dragging=%v item=%+v", a.treeDragging, a.treeDragItem)
	}
	if _, ok := tt.Find(locales.TreeDragProjectionLabel("source.md")); !ok {
		t.Fatal("projection card missing before target expansion")
	}
	tt.Advance(650 * time.Millisecond)
	tt.Frame()
	if !a.expanded[folder] {
		t.Fatal("collapsed target did not expand after hover delay")
	}
	assertProjectionFollowsPointer := func(stage string) {
		t.Helper()
		projection, ok := tt.Find(locales.TreeDragProjectionLabel("source.md"))
		if !ok {
			t.Fatalf("%s projection missing: texts=%q", stage, strings.Join(tt.Texts(), "\n"))
		}
		if dx, dy := projection.X-(a.treeDragPointer.X+12), projection.Y-(a.treeDragPointer.Y-58); math.Abs(float64(dx)) > 1 || math.Abs(float64(dy)) > 1 {
			t.Fatalf("%s projection stopped following pointer: projection=%+v pointer=%+v delta=(%g,%g)", stage, projection, a.treeDragPointer, dx, dy)
		}
	}
	for i := 0; i < 3; i++ {
		tt.Frame()
		if !a.treeDragging || a.treeDragItem.Path != source || a.treeDragStarted.IsZero() {
			t.Fatalf("drag session changed after row rebuild: dragging=%v item=%+v started=%v", a.treeDragging, a.treeDragItem, a.treeDragStarted)
		}
		assertProjectionFollowsPointer(fmt.Sprintf("expansion frame %d", i))
		if _, ok := tt.Find(treeDragHandleLabel("source.md")); !ok {
			t.Fatalf("source row disappeared after expansion frame %d", i)
		}
	}
	for index, x := range []float32{target.X + 2, target.X + target.W - 2} {
		tt.Move(x, targetY)
		tt.Frame()
		assertProjectionFollowsPointer(fmt.Sprintf("expanded folder horizontal move %d", index))
	}
	tt.Move(600, 360)
	tt.Frame()
	assertProjectionFollowsPointer("move outside tree to editor")
	tt.Release(600, 360)
	tt.Frame()
	tt.Frame()
	if !a.treeDragStarted.IsZero() || a.treePointerReleased {
		t.Fatalf("detached drag state did not clear after outside release: started=%v released=%v", a.treeDragStarted, a.treePointerReleased)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("outside release unexpectedly moved source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "source.md")); !os.IsNotExist(err) {
		t.Fatalf("outside release committed stale expanded-folder target: %v", err)
	}
}
