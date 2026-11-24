package shell

import (
	"reflect"
	"testing"
	"time"

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

func TestPanelResizeHandleDoesNotAddCanvasGap(t *testing.T) {
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame { return Frame{PanelOpen: true, PanelWidth: 248} },
		RenderCanvas: func(c *ui.Context, _ bool) {
			ui.Box(c).Key("canvas-marker").Label("canvas-marker").Size(1, 1)
		},
	})
	tester := ui.NewTester(layout.Render, 1000, 700)
	tester.Frame()
	canvas, ok := tester.Find("canvas-marker")
	if !ok || canvas.X != 257 {
		t.Fatalf("canvas x = %+v, want 257px (panel width plus canvas inset and border)", canvas)
	}
}

func TestCanvasInsetsTitleBarControlsAfterPanelCloses(t *testing.T) {
	open, width := true, float32(248)
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame { return Frame{PanelOpen: open, PanelWidth: width} },
		RenderCanvas: func(c *ui.Context, _ bool) {
			ui.Box(c).Key("canvas-marker").Label("canvas-marker").Size(1, 1)
		},
	})
	tester := ui.NewTester(layout.Render, 1000, 700)
	tester.SetTitleBar(ui.TitleBar{Left: 100})
	tester.Frame()
	opened, ok := tester.Find("canvas-marker")
	if !ok || opened.X != 257 {
		t.Fatalf("open canvas x = %+v, want 257px", opened)
	}
	open = false
	tester.Frame()
	tester.Advance(220 * time.Millisecond)
	closed, ok := tester.Find("canvas-marker")
	if !ok || closed.X != 53 {
		t.Fatalf("closed canvas x = %+v, want 53px (8px canvas inset plus title-bar space beyond the 56px control area)", closed)
	}
}

func TestCollapsedCardPanelDoesNotLeaveAResizeHandleOnCanvas(t *testing.T) {
	layout := NewLayout(LayoutPorts{
		Frame:        func() Frame { return Frame{PanelOpen: false, PanelWidth: 248} },
		RenderCanvas: func(*ui.Context, bool) {},
	})
	tester := ui.NewTester(layout.Render, 1000, 700)
	tester.Frame()
	if edge, ok := tester.Find("调整面板宽度"); ok {
		t.Fatalf("collapsed card panel left a canvas resize target at %+v; main renders this handle only while the panel is present", edge)
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

func TestVisibleResizeGripCanDragPanelClosed(t *testing.T) {
	open, width := true, float32(248)
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame {
			return Frame{PanelOpen: open, PanelWidth: width, PanelResizeTop: 53}
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
	if !ok || edge.Y != 53 {
		t.Fatalf("resize edge = %+v, want top inset 53px", edge)
	}
	tester.Move(edge.X+edge.W/2, edge.Y+edge.H/2)
	tester.Frame()
	grip, ok := tester.Find("侧栏把手")
	if !ok || grip.W == 0 || grip.H == 0 {
		t.Fatalf("visible resize grip = %+v, want a hit target", grip)
	}
	x, y := grip.X+grip.W/2, grip.Y+grip.H/2
	tester.Press(x, y)
	tester.Frame()
	tester.Move(x-220, y)
	tester.Frame()
	tester.Release(x-220, y)
	tester.Frame()
	if open || width != 248 {
		t.Fatalf("dragging visible grip left kept panel open=%v width=%.1f, want folded while retaining width", open, width)
	}
}

func TestPanelResizeCanStartBesideVisibleGrip(t *testing.T) {
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
	edge, ok := tester.Find("调整面板宽度")
	if !ok || edge.W != 12 {
		t.Fatalf("resize hit target = %+v, want main's 12px panel edge", edge)
	}
	// Start near the editor side of the edge, outside the visible 2px grip.
	x, y := edge.X+edge.W-2, edge.Y+edge.H/2
	tester.Press(x, y)
	tester.Frame()
	tester.Move(x-80, y)
	tester.Frame()
	tester.Release(x-80, y)
	tester.Frame()
	if open {
		t.Fatalf("drag beside the visible grip did not collapse panel; width=%.1f", width)
	}
}

func TestFlatResizeHandleAnchorsToEditorEdge(t *testing.T) {
	open, width := true, float32(248)
	layout := NewLayout(LayoutPorts{
		Frame:        func() Frame { return Frame{PanelOpen: open, PanelWidth: width, FlatCanvas: true} },
		RenderCanvas: func(*ui.Context, bool) {},
		Dispatch: func(action Action) {
			if action.Kind == PanelGeometryChanged {
				open, width = action.PanelOpen, action.PanelWidth
			}
		},
	})
	tester := ui.NewTester(layout.Render, 1000, 700)
	tester.Frame()
	edge, ok := tester.Find("调整面板宽度")
	if !ok || edge.X != 248 {
		t.Fatalf("flat resize hit target = %+v, want editor edge at x=248", edge)
	}
	x, y := edge.X+edge.W/2, edge.Y+edge.H/2
	tester.Press(x, y)
	tester.Frame()
	tester.Move(x-80, y)
	tester.Frame()
	tester.Release(x-80, y)
	tester.Frame()
	if open {
		t.Fatalf("drag from editor-edge hit target did not collapse panel; width=%.1f", width)
	}
}

func TestFlatPanelContinuousDragTracksEachMove(t *testing.T) {
	open, width := true, float32(248)
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame {
			return Frame{PanelOpen: open, PanelWidth: width, FlatCanvas: true, PanelMinWidth: 220, PanelMaxWidth: 560}
		},
		RenderCanvas: func(*ui.Context, bool) {},
		Dispatch: func(action Action) {
			if action.Kind == PanelGeometryChanged {
				open, width = action.PanelOpen, action.PanelWidth
			}
		},
	})
	tester := ui.NewTester(layout.Render, 1200, 800)
	tester.Frame()
	edge, _ := tester.Find("调整面板宽度")
	x, y, start := edge.X+edge.W/2, edge.Y+40, width
	tester.Press(x, y)
	tester.Frame()
	for _, delta := range []float32{10, 30, 50, 25} {
		tester.Move(x+delta, y)
		tester.Frame()
		tester.Frame()
		if width != start+delta {
			t.Fatalf("delta=%v width=%v, want %v", delta, width, start+delta)
		}
	}
	tester.Release(x+25, y)
	tester.Frame()
}

func TestFlatPanelSlowLeftDragTracksBeforeFoldThreshold(t *testing.T) {
	open, width := true, float32(248)
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame {
			return Frame{PanelOpen: open, PanelWidth: width, FlatCanvas: true, PanelMinWidth: 224, PanelFoldAt: 200}
		},
		RenderCanvas: func(*ui.Context, bool) {},
		Dispatch: func(action Action) {
			if action.Kind == PanelGeometryChanged {
				open, width = action.PanelOpen, action.PanelWidth
			}
		},
	})
	tester := ui.NewTester(layout.Render, 1200, 800)
	tester.Frame()
	edge, ok := tester.Find("调整面板宽度")
	if !ok {
		t.Fatal("panel resize edge is missing")
	}
	x, y, start := edge.X+edge.W/2, edge.Y+40, width
	tester.Press(x, y)
	tester.Frame()
	for _, delta := range []float32{-8, -16, -24, -32, -40, -48, -56} {
		tester.Move(x+delta, y)
		tester.Frame()
		tester.Frame()
		if delta > -56 && (!open || width != max(start+delta, 224)) {
			t.Fatalf("slow delta=%v open=%v width=%v, want open width=%v", delta, open, width, max(start+delta, 224))
		}
		if delta == -56 && (open || width != 224) {
			t.Fatalf("slow delta=%v open=%v width=%v, want folded after crossing threshold", delta, open, width)
		}
	}
	tester.Release(x-56, y)
	tester.Frame()
}

func TestFlatPanelSlowDragAndReverseWaitsForHandle(t *testing.T) {
	open, width := true, float32(248)
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame {
			return Frame{PanelOpen: open, PanelWidth: width, FlatCanvas: true, PanelMinWidth: 224, PanelFoldAt: 200, PanelOpenAt: 36}
		},
		RenderCanvas: func(*ui.Context, bool) {},
		Dispatch: func(action Action) {
			if action.Kind == PanelGeometryChanged {
				open, width = action.PanelOpen, action.PanelWidth
			}
		},
	})
	tester := ui.NewTester(layout.Render, 1200, 800)
	tester.Frame()
	edge, _ := tester.Find("调整面板宽度")
	x, y := edge.X+edge.W/2, edge.Y+40
	tester.Press(x, y)
	tester.Frame()
	for _, delta := range []float32{-8, -16, -24, -32, -40, -48, -100} {
		tester.Move(x+delta, y)
		tester.Frame()
	}
	if open {
		t.Fatal("slow left drag did not fold at the configured threshold")
	}
	tester.Move(x-64, y)
	tester.Frame()
	if !open {
		t.Fatal("reverse drag did not open the panel at the configured threshold")
	}
	widthAfterOpen := width
	if widthAfterOpen != 224 {
		t.Fatalf("reverse open changed the clamped panel width: %v", widthAfterOpen)
	}
	tester.Move(widthAfterOpen-4, y)
	tester.Frame()
	if width != widthAfterOpen {
		t.Fatalf("panel resized before the handle caught the pointer: got %v want %v", width, widthAfterOpen)
	}
	tester.Move(widthAfterOpen+6, y)
	tester.Frame()
	if width != widthAfterOpen {
		t.Fatalf("catching the handle changed panel width: got %v want %v", width, widthAfterOpen)
	}
	tester.Move(widthAfterOpen+26, y)
	tester.Frame()
	if width <= widthAfterOpen {
		t.Fatalf("panel did not resume resizing after handle catch-up: got %v start %v", width, widthAfterOpen)
	}
	tester.Release(widthAfterOpen+26, y)
	tester.Frame()
}

func TestCardPanelSlowDragAndReverseWaitsForHandle(t *testing.T) {
	open, width := true, float32(248)
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame {
			return Frame{PanelOpen: open, PanelWidth: width, PanelMinWidth: 224, PanelFoldAt: 200, PanelOpenAt: 36}
		},
		Dispatch: func(action Action) {
			if action.Kind == PanelGeometryChanged {
				open, width = action.PanelOpen, action.PanelWidth
			}
		},
	})
	tester := ui.NewTester(layout.Render, 1200, 800)
	tester.Frame()
	edge, ok := tester.Find("调整面板宽度")
	if !ok {
		t.Fatal("panel resize edge is missing")
	}
	x, y := edge.X+edge.W/2, edge.Y+edge.H/2
	tester.Press(x, y)
	tester.Frame()
	for _, delta := range []float32{-8, -16, -24, -32, -40, -48, -56} {
		tester.Move(x+delta, y)
		tester.Frame()
		if delta > -56 && !open {
			t.Fatalf("slow drag folded before crossing threshold at delta=%v", delta)
		}
	}
	if open {
		t.Fatal("slow left drag did not fold at the configured threshold")
	}
	tester.Move(x-12, y)
	tester.Frame()
	if !open {
		t.Fatal("reverse drag did not open the panel after crossing its threshold")
	}
	openedWidth := width
	if openedWidth != 224 {
		t.Fatalf("reverse open width = %v, want clamped width 224", openedWidth)
	}
	for _, delta := range []float32{openedWidth - 4, openedWidth + 6} {
		tester.Move(delta, y)
		tester.Frame()
		if width != openedWidth {
			t.Fatalf("panel resized before its animated edge caught the pointer at x=%v: %v -> %v", delta, openedWidth, width)
		}
	}
	tester.Move(openedWidth+26, y)
	tester.Frame()
	if width <= openedWidth {
		t.Fatalf("resize did not resume after the edge caught the pointer: width=%v start=%v", width, openedWidth)
	}
	tester.Release(openedWidth+26, y)
	tester.Frame()
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
	tester.Advance(50 * time.Millisecond)
	grip, _ = tester.Find("侧栏把手")
	if grip.H <= 40 || grip.H >= 56 {
		t.Fatalf("held grip spring = %+v, want an intermediate size", grip)
	}
	tester.Advance(200 * time.Millisecond)
	grip, _ = tester.Find("侧栏把手")
	if grip.H < 55 || grip.H > 57 {
		t.Fatalf("held grip = %+v, want 56px", grip)
	}
}

func TestPanelToggleKeepsItsPositionAcrossOpenState(t *testing.T) {
	open := true
	icon := ui.MustParseSVG([]byte(`<svg width="16" height="16" viewBox="0 0 16 16"><path d="M2 8h12"/></svg>`))
	layout := NewLayout(LayoutPorts{
		Frame: func() Frame {
			return Frame{PanelOpen: open, PanelWidth: 248, PanelToggleLabel: "sidebar", PanelToggleIcon: icon}
		},
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
		Frame:        func() Frame { return Frame{PanelOpen: open, PanelWidth: width, FlatCanvas: true} },
		RenderCanvas: func(*ui.Context, bool) {},
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
	for _, delta := range []float32{20, 40, 80, 60} {
		tester.Move(x+delta, y)
		tester.Frame()
		tester.Advance(200 * time.Millisecond)
	}
	tester.Release(x+60, y)
	tester.Frame()
	if !open || width < 220 || width > 560 {
		t.Fatalf("reopened panel open=%v width=%.1f, want an open panel within resize bounds", open, width)
	}
}

func TestCollapsedPanelWaitsForPointerAtFinalEdgeBeforeResize(t *testing.T) {
	open, width := false, float32(248)
	layout := NewLayout(LayoutPorts{
		Frame:        func() Frame { return Frame{PanelOpen: open, PanelWidth: width, FlatCanvas: true, PanelOpenAt: 36} },
		RenderCanvas: func(*ui.Context, bool) {},
		Dispatch: func(action Action) {
			if action.Kind == PanelGeometryChanged {
				open, width = action.PanelOpen, action.PanelWidth
			}
		},
	})
	tester := ui.NewTester(layout.Render, 1000, 700)
	tester.Frame()
	edge, _ := tester.Find("调整面板宽度")
	x, y := edge.X+edge.W/2, edge.Y+40
	tester.Press(x, y)
	tester.Frame()
	tester.Move(x+40, y)
	tester.Frame()
	if !open || !layout.awaitingFinalEdge {
		t.Fatalf("after pull threshold open=%v awaiting final edge=%v", open, layout.awaitingFinalEdge)
	}
	if width != 248 {
		t.Fatalf("before pointer reaches target edge width=%v, want 248", width)
	}
	tester.Move(248, y)
	tester.Frame()
	if layout.awaitingFinalEdge || !open || width != 248 {
		t.Fatalf("at final edge open=%v awaiting=%v width=%v", open, layout.awaitingFinalEdge, width)
	}
	tester.Release(248, y)
	tester.Frame()
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
