package nativeui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPanelDragResizesAndFolds(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	if !a.panelOpen {
		t.Fatal("panel starts closed")
	}
	r, ok := tt.Find("调整面板宽度")
	if !ok || r.W == 0 {
		t.Fatalf("the resize handle is not on screen: %+v", r)
	}
	x, y := r.X+r.W/2, r.Y+40

	// A drag left past the fold: the width clamps at the minimum while the
	// pointer holds, and releasing folds the panel away.
	tt.Press(x, y)
	tt.Frame()
	tt.Move(x-100, y)
	tt.Frame()
	if a.panelWidth != panelMinWidth {
		t.Fatalf("width = %.0f, want the minimum %.0f", a.panelWidth, panelMinWidth)
	}
	tt.Release(x-100, y)
	tt.Frame()
	if a.panelOpen {
		t.Fatal("a drag past the fold kept the panel open")
	}

	// A drag inside the bounds only moves the edge, and the panel stays.
	a2 := New()
	tt2 := ui.NewTester(a2.View, 1200, 800)
	tt2.Frame()
	r2, ok := tt2.Find("调整面板宽度")
	if !ok || r2.W == 0 {
		t.Fatalf("the resize handle is not on screen: %+v", r2)
	}
	x, y = r2.X+r2.W/2, r2.Y+40
	tt2.Press(x, y)
	tt2.Frame()
	tt2.Move(x-20, y)
	tt2.Frame()
	tt2.Release(x-20, y)
	tt2.Frame()
	if !a2.panelOpen {
		t.Fatal("a small drag folded the panel")
	}
	if w := a2.panelWidth; w != 248 {
		t.Fatalf("width = %.0f, want 248", w)
	}
}

// TestModulesShareOnePanelWidth checks that switching modules never
// resizes the panel: the four views share one width, so the editor beside
// it does not reflow and flicker.
func TestModulesShareOnePanelWidth(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	want := a.panelWidth
	if want != panelDefaultWidth {
		t.Fatalf("the panel starts at %.0f, want the one %.0f", want, panelDefaultWidth)
	}
	for _, label := range []string{"大纲", "搜索", "AI 助手", "文件"} {
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %s: %v", label, err)
		}
		tt.Frame()
		if a.panelWidth != want {
			t.Fatalf("switching to %s set the panel to %.0f, want the shared %.0f", label, a.panelWidth, want)
		}
		if a.panelW != want {
			t.Fatalf("switching to %s drew the panel %.0f wide, want %.0f", label, a.panelW, want)
		}
	}
	// A width the user dragged to stays across a switch as well.
	a.panelWidth = 320
	if err := tt.Click("AI 助手"); err != nil {
		t.Fatalf("click AI: %v", err)
	}
	tt.Frame()
	if a.panelWidth != 320 {
		t.Fatalf("switching to AI set the dragged width to %.0f, want 320", a.panelWidth)
	}
}

func TestCollapsedPanelReopensFromTheHeader(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("打开侧栏"); ok {
		t.Fatal("the reopen icon shows while the panel is open")
	}
	a.collapsePanel()
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("打开侧栏"); !ok {
		t.Fatal("no reopen icon while the panel is folded")
	}
	if err := tt.Click("打开侧栏"); err != nil {
		t.Fatalf("click reopen: %v", err)
	}
	if !a.panelOpen {
		t.Fatal("the reopen icon did not bring the panel back")
	}
}

// TestTopBarControlsShareOneLine reads the top bar off the layout: the rail's
// module buttons, the panel header's close and the editor header's path and
// its two tools all centre on the same height, the one the window buttons sit
// on.
func TestTopBarControlsShareOneLine(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	for _, s := range []string{"文件", "收起侧栏", "未命名", "信息", "更多"} {
		r, ok := tt.Find(s)
		if !ok {
			t.Fatalf("%q is not on screen", s)
		}
		if s == "文件" && r.X >= railWidth {
			continue // the panel's title shares the word; the rail's button is meant
		}
		if d := r.Y + r.H/2 - topLineCenter; d < -0.5 || d > 0.5 {
			t.Errorf("%q centres at %.2f, want the top line at %.1f", s, r.Y+r.H/2, topLineCenter)
		}
	}
}
