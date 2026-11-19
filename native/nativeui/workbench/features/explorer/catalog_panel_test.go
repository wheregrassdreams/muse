package explorer

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestRootLabelUsesVaultNameAndPreservesDotRoot(t *testing.T) {
	for _, test := range []struct{ root, want string }{
		{root: "/notes/work", want: "work"},
		{root: ".", want: "."},
	} {
		if got := RootLabel(test.root); got != test.want {
			t.Errorf("RootLabel(%q) = %q, want %q", test.root, got, test.want)
		}
	}
}

func TestCatalogPanelDirectoryTitleClickAndDrag(t *testing.T) {
	const folder = "/vault/Projects"
	controller := NewController()
	var actions []Action
	ports := CatalogPanelPorts{
		Root: "/vault", RootLabel: "vault", Controller: controller,
		Children: func(path string) []CatalogEntry {
			if path == "/vault" {
				return []CatalogEntry{{Path: "Projects", Kind: CatalogDirectory}}
			}
			if path == folder {
				return []CatalogEntry{{Path: "Projects/Overview.md", Kind: CatalogFile}}
			}
			return nil
		},
		IsDirectory: func(path string) bool { return path == folder },
		Drag: DragPort{
			Make: func(path string, ids []string) DragItem {
				return DragItem{Path: path, Paths: append([]string(nil), ids...)}
			},
			IDs: func(item DragItem) []string { return append([]string(nil), item.Paths...) },
		},
		DragPreview: &DragSnapshot{ItemName: "Projects", DisplayID: "Projects", Directory: true},
		OnAction:    func(action Action) { actions = append(actions, action) },
	}
	tester := ui.NewTester(func(c *ui.Context) { RenderCatalogPanel(c, ports) }, 360, 180)
	tester.Frame()
	row, ok := tester.Find("Projects")
	if !ok {
		t.Fatalf("directory row missing: %v", tester.Texts())
	}
	tester.ClickAt(row.X+row.W/2, row.Y+row.H/2)
	tester.Frame()
	if !controller.IsExpanded(folder) {
		t.Fatal("clicking the directory title did not expand the real catalog panel")
	}
	row, ok = tester.Find("Projects")
	if !ok {
		t.Fatalf("expanded directory row missing: %v", tester.Texts())
	}
	tester.ClickAt(row.X+row.W/2, row.Y+row.H/2)
	tester.Frame()
	if controller.IsExpanded(folder) {
		t.Fatal("clicking the directory title did not collapse the real catalog panel")
	}
	actions = nil
	row, ok = tester.Find("Projects")
	if !ok {
		t.Fatalf("collapsed directory row missing before drag: %v", tester.Texts())
	}
	x, y := row.X+row.W/2, row.Y+row.H/2
	tester.Press(x, y)
	tester.Move(x+20, y)
	tester.Frame()
	started := false
	for _, action := range actions {
		if action.Kind == StartDrag && action.ID == folder && action.Payload.Path == folder {
			started = true
		}
	}
	if !started || !controller.DragPreview().Active {
		t.Fatalf("dragging the directory title did not start the real catalog drag: started=%v preview=%+v actions=%+v", started, controller.DragPreview(), actions)
	}
	tester.Frame()
	projectionLabel := locales.TreeDragProjectionLabel("Projects")
	projections := 0
	for _, text := range tester.Texts() {
		if text == projectionLabel {
			projections++
		}
	}
	if projections != 1 {
		t.Fatalf("real catalog drag rendered %d projection cards, want one (%q): %v", projections, projectionLabel, tester.Texts())
	}
}
