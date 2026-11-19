package workspaceui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	hosting "muse/nativeui/workbench/framework/hosting"
	"muse/workspace"
)

type presenterTestContent struct{ rendered int }

func (c *presenterTestContent) Render(*ui.Context) { c.rendered++ }
func (*presenterTestContent) Close()               {}

func TestRenderRecursivelyRendersOnlyActiveTabPanes(t *testing.T) {
	host := hosting.NewManager(nil)
	left, right, inactive := &presenterTestContent{}, &presenterTestContent{}, &presenterTestContent{}
	handles := make(map[hosting.PaneID]hosting.Handle)
	for _, item := range []struct {
		pane    hosting.PaneID
		content *presenterTestContent
	}{
		{pane: "left", content: left},
		{pane: "right", content: right},
		{pane: "other", content: inactive},
	} {
		handle, err := host.Open(item.content, hosting.Placement{Host: "main", Pane: item.pane, Layer: hosting.PaneContent})
		if err != nil {
			t.Fatal(err)
		}
		handles[item.pane] = handle
	}
	state := workspace.State{
		Version: 1, ActiveTab: "work",
		Tabs: []workspace.Tab{
			{
				ID: "work", ActivePaneID: "left",
				Layout: workspace.Branch(workspace.Horizontal, .4,
					workspace.Leaf(workspace.Pane{ID: "left", Document: workspace.Document{Path: "/left.md"}}),
					workspace.Branch(workspace.Vertical, .6,
						workspace.Leaf(workspace.Pane{ID: "right", Document: workspace.Document{Path: "/right.md"}}),
						workspace.Leaf(workspace.Pane{ID: "third", Document: workspace.Document{Path: "/third.md"}}),
					),
				),
			},
			{ID: "other-tab", ActivePaneID: "other", Layout: workspace.Leaf(workspace.Pane{ID: "other", Document: workspace.Document{Path: "/other.md"}})},
		},
	}
	third := &presenterTestContent{}
	thirdHandle, err := host.Open(third, hosting.Placement{Host: "main", Pane: "third", Layer: hosting.PaneContent})
	if err != nil {
		t.Fatal(err)
	}
	handles["third"] = thirdHandle

	presenter := newPresenter()
	var focusedAction Action
	tester := ui.NewTester(func(c *ui.Context) {
		presenter.Render(c, state, "main", host.RenderPane, func(action Action) {
			focusedAction = action
			if action.Kind == FocusPane {
				host.Focus(handles[hosting.PaneID(action.PaneID)])
			}
		})
	}, 800, 600)
	tester.Frame()

	if left.rendered == 0 || right.rendered == 0 || third.rendered == 0 {
		t.Fatalf("active panes did not all render: left=%d right=%d third=%d", left.rendered, right.rendered, third.rendered)
	}
	if inactive.rendered != 0 {
		t.Fatalf("inactive tab rendered %d times", inactive.rendered)
	}
	tester.ClickAt(100, 100)
	if focusedAction.Kind != FocusPane || focusedAction.PaneID != "left" {
		t.Fatalf("click action = %+v, want focus of left pane", focusedAction)
	}
	if pane, ok := host.FocusedPane("main"); !ok || pane != "left" {
		t.Fatalf("manager focused pane = %q, %v after click; want left", pane, ok)
	}
}
