package workspaceui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	appworkspace "muse/application/workspace"
	domain "muse/workspace"
)
func TestHostRestoreOpensActiveTabPanesAndRestoresFocus(t *testing.T) {
	manager := NewManager(nil)
	var requests []PaneRequest
	host := NewHost(Ports{
		Windows: manager,
		OpenPane: func(request PaneRequest) (Handle, error) {
			requests = append(requests, request)
			content := &testContent{}
			return manager.Open(content, Placement{Host: MainHost, Pane: PaneID(request.PaneID), Layer: PaneContent})
		},
	})
	plan := appworkspace.RestorePlan{
		ActiveTab: "main", ActivePane: "right",
		Documents: []appworkspace.DocumentRestore{
			{TabID: "main", PaneID: "left", Path: "/notes/left.md", Cursor: 3},
			{TabID: "main", PaneID: "right", Path: "/notes/right.md", Cursor: 8, Focused: true},
			{TabID: "later", PaneID: "later-pane", Path: "/notes/later.md", Cursor: 13},
		},
	}
	if err := host.OpenRestorePlan(plan); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[0].PaneID != "left" || requests[1].PaneID != "right" {
		t.Fatalf("opened pane requests = %+v, want active tab only", requests)
	}
	if requests[0].Path != "/notes/left.md" || requests[0].CursorOffset != 3 || requests[1].Path != "/notes/right.md" || requests[1].CursorOffset != 8 {
		t.Fatalf("restore document details = %+v", requests)
	}
	if pane, ok := manager.FocusedPane(MainHost); !ok || pane != "right" {
		t.Fatalf("focused pane = %q, %t; want right", pane, ok)
	}
	if host.PaneHandle("left").IsZero() || host.PaneHandle("right").IsZero() || !host.PaneHandle("later-pane").IsZero() {
		t.Fatalf("registered pane slots = left:%v right:%v later:%v", host.PaneHandle("left"), host.PaneHandle("right"), host.PaneHandle("later-pane"))
	}
	host.ClosePaneSlots()
	if !host.PaneHandle("left").IsZero() || !host.PaneHandle("right").IsZero() {
		t.Fatal("closing pane slots retained handles")
	}
}

func TestHostRestoreUsesApplicationDefaultTabPlan(t *testing.T) {
	manager := NewManager(nil)
	var requests []PaneRequest
	host := NewHost(Ports{
		Windows: manager,
		OpenPane: func(request PaneRequest) (Handle, error) {
			requests = append(requests, request)
			return manager.Open(&testContent{}, Placement{Host: MainHost, Pane: PaneID(request.PaneID), Layer: PaneContent})
		},
	})
	plan := appworkspace.RestorePlan{DefaultTab: appworkspace.DefaultTabPlan{
		Decision: appworkspace.CreateBlankTab, TabID: appworkspace.DefaultTabID, PaneID: appworkspace.DefaultPaneID,
	}}
	if err := host.OpenRestorePlan(plan); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || requests[0].TabID != "main" || requests[0].PaneID != "editor" || requests[0].Path != "" || !requests[0].Focused {
		t.Fatalf("default pane requests = %+v", requests)
	}
}

func TestHostOpenPaneReplacesLiveSurfaceInSameSlot(t *testing.T) {
	manager := NewManager(nil)
	opened := 0
	host := NewHost(Ports{
		Windows: manager,
		OpenPane: func(request PaneRequest) (Handle, error) {
			opened++
			content := &testContent{}
			return manager.Open(content, Placement{Host: MainHost, Pane: PaneID(request.PaneID), Layer: PaneContent})
		},
	})
	first, err := host.OpenPane(PaneRequest{TabID: appworkspace.DefaultTabID, PaneID: appworkspace.DefaultPaneID, Focused: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := host.OpenPane(PaneRequest{TabID: appworkspace.DefaultTabID, PaneID: appworkspace.DefaultPaneID, Path: "/notes/next.md", Focused: true})
	if err != nil {
		t.Fatal(err)
	}
	if opened != 2 || first == second || manager.Contains(first) || host.PaneHandle(appworkspace.DefaultPaneID) != second {
		t.Fatalf("open count=%d handles=(%v,%v) slot=%v", opened, first, second, host.PaneHandle(appworkspace.DefaultPaneID))
	}
}

func TestHostRegisterReplaceReleaseAndFocusPaneSlots(t *testing.T) {
	manager := NewManager(nil)
	host := NewHost(Ports{Windows: manager})
	firstContent, secondContent := &testContent{}, &testContent{}
	first, err := manager.Open(firstContent, Placement{Host: MainHost, Pane: "editor", Layer: PaneContent})
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Open(secondContent, Placement{Host: MainHost, Pane: "editor", Layer: PaneContent})
	if err != nil {
		t.Fatal(err)
	}
	host.RegisterPane("editor", first)
	host.RegisterPane("editor", second)
	if manager.Contains(first) || host.PaneHandle("editor") != second {
		t.Fatal("replacing pane slot did not close and replace the old handle")
	}
	if !manager.Focus(second) || host.FocusedPane("") != "editor" {
		t.Fatal("focused pane did not resolve from the window host")
	}
	host.ReleasePane("editor")
	if !host.PaneHandle("editor").IsZero() || !manager.Contains(second) {
		t.Fatal("release should forget the slot without closing its handle")
	}
	host.RegisterPane("editor", second)
	if err := host.ClosePaneSlot("editor"); err != nil {
		t.Fatal(err)
	}
	if manager.Contains(second) || !host.PaneHandle("editor").IsZero() {
		t.Fatal("closing pane slot should close the handle and clear its mapping")
	}
}

func TestHostRenderForwardsFocusActionsAndPaneContent(t *testing.T) {
	manager := NewManager(nil)
	content := &testContent{}
	handle, err := manager.Open(content, Placement{Host: MainHost, Pane: "editor", Layer: PaneContent})
	if err != nil {
		t.Fatal(err)
	}
	host := NewHost(Ports{Windows: manager})
	host.RegisterPane("editor", handle)
	state := domain.State{Version: 1, ActiveTab: "main", Tabs: []domain.Tab{{
		ID: "main", ActivePaneID: "editor",
		Layout: domain.Leaf(domain.Pane{ID: "editor", Document: domain.Document{Path: "/note.md"}}),
	}}}
	var actions []Action
	tester := ui.NewTester(func(c *ui.Context) {
		host.Render(c, state, func(action Action) { actions = append(actions, action) })
	}, 800, 600)
	tester.Frame()
	if content.rendered == 0 {
		t.Fatal("host did not project pane content")
	}
	tester.ClickAt(80, 80)
	if len(actions) == 0 || actions[len(actions)-1].Kind != FocusPane || actions[len(actions)-1].PaneID != "editor" {
		t.Fatalf("focus actions = %+v", actions)
	}
}

func TestHostRenderShowsDefaultPaneBeforeWorkspaceHasDurableTab(t *testing.T) {
	manager := NewManager(nil)
	content := &testContent{}
	handle, err := manager.Open(content, Placement{Host: MainHost, Pane: PaneID(appworkspace.DefaultPaneID), Layer: PaneContent})
	if err != nil {
		t.Fatal(err)
	}
	host := NewHost(Ports{Windows: manager})
	host.RegisterPane(appworkspace.DefaultPaneID, handle)
	tester := ui.NewTester(func(c *ui.Context) {
		host.Render(c, domain.State{Version: 1}, nil)
	}, 800, 600)
	tester.Frame()
	if content.rendered == 0 {
		t.Fatal("host did not render the default blank pane without a durable active tab")
	}
}
