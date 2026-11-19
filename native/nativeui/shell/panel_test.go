package shell

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLayoutComposesSurfacesThroughPorts(t *testing.T) {
	var order []string
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame {
			order = nil
			return Frame{PanelOpen: true, PanelWidth: 248, Surface: SurfaceAgent, Background: ui.Hex("#fff")}
		},
		RenderPanel: func(_ *ui.Context, surface Surface) {
			if surface != SurfaceAgent {
				t.Errorf("panel surface = %v, want agent", surface)
			}
			order = append(order, "panel")
		},
		RenderCanvas:    func(*ui.Context, bool) { order = append(order, "canvas") },
		RenderInspector: func(*ui.Context) { order = append(order, "inspector") },
		RenderOverlays:  func(*ui.Context) { order = append(order, "overlays") },
	})
	tester := ui.NewTester(layout.Render, 1000, 700)
	tester.Frame()
	if !reflect.DeepEqual(order, []string{"panel", "canvas", "inspector", "overlays"}) {
		t.Fatalf("surface order = %v", order)
	}
}

func TestLayoutOmitsVerticalSurfaceRail(t *testing.T) {
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame {
			return Frame{PanelOpen: true, PanelWidth: 248, Surface: SurfaceFiles}
		},
		RenderPanel:  func(*ui.Context, Surface) {},
		RenderCanvas: func(*ui.Context, bool) {},
	})
	tester := ui.NewTester(layout.Render, 800, 600)
	tester.Frame()
	if _, ok := tester.Find("shell-rail-files"); ok {
		t.Fatal("vertical surface rail is still rendered")
	}
}

func TestPanelResizeDispatchesGeometryChanges(t *testing.T) {
	open, width := true, float32(248)
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame {
			return Frame{PanelOpen: open, PanelWidth: width, PanelMinWidth: 220, PanelMaxWidth: 560}
		},
		Dispatch: func(action Action) {
			if action.Kind == PanelGeometryChanged {
				open, width = action.PanelOpen, action.PanelWidth
			}
		},
	})
	tester := ui.NewTester(layout.Render, 1000, 700)
	tester.Frame()
	edge, ok := tester.Find("调整面板宽度")
	if !ok {
		t.Fatal("panel resize target is missing")
	}
	x, y := edge.X+edge.W/2, edge.Y+edge.H/2
	tester.Press(x, y)
	tester.Frame()
	tester.Move(x+24, y)
	tester.Frame()
	tester.Release(x+24, y)
	tester.Frame()
	if !open || width < 260 || width > 280 {
		t.Fatalf("resize result open=%v width=%.1f, want open and about 272px", open, width)
	}
}

func TestDraggingPastFoldThresholdCollapsesPanel(t *testing.T) {
	open, width := true, float32(248)
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame { return Frame{PanelOpen: open, PanelWidth: width} },
		Dispatch: func(action Action) {
			if action.Kind == PanelGeometryChanged {
				open, width = action.PanelOpen, action.PanelWidth
			}
		},
	})
	tester := ui.NewTester(layout.Render, 1000, 700)
	tester.Frame()
	edge, _ := tester.Find("调整面板宽度")
	x, y := edge.X+edge.W/2, edge.Y+edge.H/2
	tester.Press(x, y)
	tester.Frame()
	tester.Move(x-220, y)
	tester.Frame()
	tester.Release(x-220, y)
	tester.Frame()
	if open {
		t.Fatalf("panel stayed open after crossing fold threshold; width=%.1f", width)
	}
}

func TestResizeGripExpandsOnHoverAndHold(t *testing.T) {
	layout := NewLayout(LayoutPorts{Frame: func() Frame { return Frame{PanelOpen: true, PanelWidth: 248} }})
	tester := ui.NewTester(layout.Render, 1000, 700)
	tester.Frame()
	edge, ok := tester.Find("调整面板宽度")
	if !ok {
		t.Fatal("panel resize edge is missing")
	}
	grip, ok := tester.Find("侧栏把手")
	if !ok || grip.H > 10 {
		t.Fatalf("idle grip = %+v, want collapsed", grip)
	}
	tester.Move(edge.X+edge.W/2, edge.Y+edge.H/2)
	tester.Frame()
	grip, _ = tester.Find("侧栏把手")
	if grip.H < 39 || grip.H > 41 {
		t.Fatalf("hovered grip = %+v, want 40px", grip)
	}
	tester.Press(edge.X+edge.W/2, edge.Y+edge.H/2)
	tester.Frame()
	grip, _ = tester.Find("侧栏把手")
	if grip.H < 55 || grip.H > 57 {
		t.Fatalf("held grip = %+v, want 56px", grip)
	}
}

func TestPanelToggleKeepsItsPositionAcrossOpenState(t *testing.T) {
	open := true
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame { return Frame{PanelOpen: open, PanelWidth: 248, PanelToggleLabel: "sidebar"} },
		Dispatch: func(action Action) {
			if action.Kind == TogglePanel {
				open = action.PanelOpen
			}
		},
	})
	tester := ui.NewTester(layout.Render, 1000, 700)
	tester.Frame()
	openToggle, ok := tester.Find("sidebar")
	if !ok {
		t.Fatal("panel toggle is missing")
	}
	if err := tester.Click("sidebar"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	closedToggle, ok := tester.Find("sidebar")
	if !ok || closedToggle.X != openToggle.X {
		t.Fatalf("toggle moved from x=%.1f to %+v", openToggle.X, closedToggle)
	}
	if open {
		t.Fatal("toggle click did not close panel")
	}
}

func TestPanelResizeClampsWidthToFrameLimits(t *testing.T) {
	open, width := true, float32(248)
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame {
			return Frame{PanelOpen: open, PanelWidth: width, PanelMinWidth: 220, PanelMaxWidth: 320}
		},
		Dispatch: func(action Action) {
			if action.Kind == PanelGeometryChanged {
				open, width = action.PanelOpen, action.PanelWidth
			}
		},
	})
	tester := ui.NewTester(layout.Render, 1000, 700)
	tester.Frame()
	edge, _ := tester.Find("调整面板宽度")
	x, y := edge.X+edge.W/2, edge.Y+edge.H/2
	tester.Press(x, y)
	tester.Frame()
	tester.Move(x+800, y)
	tester.Frame()
	tester.Release(x+800, y)
	tester.Frame()
	if !open || width != 320 {
		t.Fatalf("clamped resize open=%v width=%.1f, want true/320", open, width)
	}
}

func TestCollapsedPanelReopensAfterPullingPastThreshold(t *testing.T) {
	open, width := false, float32(248)
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame { return Frame{PanelOpen: open, PanelWidth: width} },
		Dispatch: func(action Action) {
			if action.Kind == PanelGeometryChanged {
				open, width = action.PanelOpen, action.PanelWidth
			}
		},
	})
	tester := ui.NewTester(layout.Render, 1000, 700)
	tester.Frame()
	edge, _ := tester.Find("调整面板宽度")
	x, y := edge.X+edge.W/2, edge.Y+edge.H/2
	tester.Press(x, y)
	tester.Frame()
	tester.Move(x+100, y)
	tester.Frame()
	tester.Release(x+100, y)
	tester.Frame()
	if !open || width < 220 || width > 560 {
		t.Fatalf("reopened panel open=%v width=%.1f, want an open panel within resize bounds", open, width)
	}
}

func TestLayoutOmitsPanelRendererWhenCollapsed(t *testing.T) {
	panelCalls := 0
	layout := NewLayout(LayoutPorts{
		Frame:       func() Frame { return Frame{PanelOpen: false, PanelWidth: 248} },
		RenderPanel: func(*ui.Context, Surface) { panelCalls++ },
	})
	tester := ui.NewTester(layout.Render, 800, 600)
	tester.Frame()
	if panelCalls != 0 {
		t.Fatalf("collapsed panel rendered %d times", panelCalls)
	}
}
