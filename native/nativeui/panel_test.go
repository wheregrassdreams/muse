package nativeui

import (
	"testing"
	"time"

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
	if a.panelOpen {
		t.Fatal("crossing fold threshold must start closing while held")
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
	for _, label := range []string{"搜索", "AI 助手", "返回文件树"} {
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

// countTexts is how many elements show s: a control that appears twice is
// the bug this counts.
func countTexts(tt *ui.Tester, s string) int {
	n := 0
	for _, text := range tt.Texts() {
		if text == s {
			n++
		}
	}
	return n
}

// TestTheSidebarToggleKeepsItsSeat: folding the panel does not move the
// control that folds it, and does not add another one. The toggle holds the
// content's left edge while the panel is open, and the editor header keeps its
// room when the panel is folded, so the page comes to the button.
func TestTheSidebarToggleKeepsItsSeat(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	open, ok := tt.Find("侧栏")
	if !ok {
		t.Fatal("no sidebar toggle")
	}
	if open.X < 2 || open.X > 16 {
		t.Fatalf("the toggle sits at %.1f, want the window's left edge at 6", open.X)
	}
	if n := countTexts(tt, "侧栏"); n != 1 {
		t.Fatalf("%d sidebar toggles, want one", n)
	}
	a.collapsePanel()
	tt.Frame()
	tt.Frame()
	folded, ok := tt.Find("侧栏")
	if !ok {
		t.Fatal("the toggle went away with the panel")
	}
	if folded.X != open.X {
		t.Fatalf("the toggle moved from %.1f to %.1f when the panel folded", open.X, folded.X)
	}
	if n := countTexts(tt, "侧栏"); n != 1 {
		t.Fatalf("%d sidebar toggles while folded, want one", n)
	}
	if err := tt.Click("侧栏"); err != nil {
		t.Fatalf("click the toggle: %v", err)
	}
	if !a.panelOpen {
		t.Fatal("the toggle did not bring the panel back")
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
	// The tester draws no title bar, so the first line is the header's own
	// height: with a title bar it is that bar's.
	line := float32(editorHeaderH / 2)
	for _, s := range []string{"侧栏", "AI 助手", "信息", "更多"} {
		r, ok := tt.Find(s)
		if !ok {
			t.Fatalf("%q is not on screen", s)
		}
		if d := r.Y + r.H/2 - line; d < -0.5 || d > 0.5 {
			t.Errorf("%q centres at %.2f, want the top line at %.1f", s, r.Y+r.H/2, line)
		}
	}
}

// Repeated motion must track the pointer rather than accumulate animation lag.
func TestFlatPanelContinuousDrag(t *testing.T) {
	a := New()
	a.flat = true
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	r, ok := tt.Find("调整面板宽度")
	if !ok || r.X < a.panelW {
		t.Fatalf("handle must be on editor side: %+v panel=%v", r, a.panelW)
	}
	x, y := r.X+r.W/2, r.Y+40
	start := a.panelWidth
	tt.Press(x, y)
	tt.Frame()
	for _, delta := range []float32{10, 30, 50, 25} {
		tt.Move(x+delta, y)
		tt.Frame()
		tt.Frame()
		if a.panelWidth != start+delta || a.panelW != a.panelWidth {
			t.Fatalf("delta=%v target=%v drawn=%v", delta, a.panelWidth, a.panelW)
		}
	}
	tt.Release(x+25, y)
	tt.Frame()
}

func TestFlatPanelHeldFoldAndReverse(t *testing.T) {
	a := New()
	a.flat = true
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	r, _ := tt.Find("调整面板宽度")
	x, y := r.X+6, r.Y+40
	start := a.panelWidth
	tt.Press(x, y)
	tt.Frame()
	for _, d := range []float32{-100, -120, -40, -10} {
		tt.Move(x+d, y)
		tt.Frame()
		if d == -100 {
			tt.Advance(30 * time.Millisecond)
			if a.panelW <= 0 || a.panelW >= start {
				t.Fatalf("missing fold intermediate frame: %v", a.panelW)
			}
		}
		tt.Advance(200 * time.Millisecond)
		wantOpen := start+d >= panelFoldOn
		if a.panelOpen != wantOpen {
			t.Fatalf("delta %v open=%v", d, a.panelOpen)
		}
		want := float32(0)
		if wantOpen {
			want = a.panelWidth
		}
		if a.panelW != want {
			t.Fatalf("delta %v drawn=%v want=%v", d, a.panelW, want)
		}
	}
	tt.Release(x-10, y)
	tt.Frame()
	tt.Frame()
	a.collapsePanel()
	tt.Frame()
	tt.Advance(30 * time.Millisecond)
	if a.panelW <= 0 || a.panelW >= start-10 {
		t.Fatalf("button collapse lost animation: %v", a.panelW)
	}
	tt.Advance(time.Second)
	if a.panelW != 0 {
		t.Fatalf("collapse residue %v", a.panelW)
	}

}

func TestFlatCollapsedEdgeDragReopens(t *testing.T) {
	a := New()
	a.flat = true
	a.panelOpen = false
	a.panelW = 0
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	r, ok := tt.Find("调整面板宽度")
	if !ok || r.X != 0 {
		t.Fatalf("missing editor edge: %+v", r)
	}
	x, y := r.X+6, r.Y+40
	tt.Move(x, y)
	tt.Frame()
	tt.Press(x, y)
	tt.Frame()
	for _, d := range []float32{20, 40, 80, 60} {
		tt.Move(x+d, y)
		tt.Frame()
		if d == 40 {
			tt.Advance(30 * time.Millisecond)
			if a.panelW <= 0 || a.panelW >= a.panelWidth {
				t.Fatalf("missing opening intermediate frame: %v", a.panelW)
			}
		}
		tt.Advance(200 * time.Millisecond)
		want := float32(0)
		if d >= 36 {
			want = panelDefaultWidth
		}
		if a.panelW != want {
			t.Fatalf("delta=%v drawn=%v want=%v", d, a.panelW, want)
		}
	}
	tt.Release(x+60, y)
	tt.Frame()
	tt.Frame()
	if !a.panelOpen || a.panelW != panelDefaultWidth {
		t.Fatalf("release open=%v width=%v", a.panelOpen, a.panelW)
	}
}

func TestEdgePullWaitsForFinalHandleBeforeResizing(t *testing.T) {
	a := New()
	a.flat = true
	a.panelOpen = false
	a.panelW = 0
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	r, _ := tt.Find("调整面板宽度")
	x, y := r.X+6, r.Y+40
	tt.Press(x, y)
	tt.Frame()
	tt.Move(x+40, y)
	tt.Frame()
	tt.Advance(30 * time.Millisecond)
	target := a.panelWidth
	if !a.panelAwaitHandle || a.panelW <= 0 || a.panelW >= target {
		t.Fatalf("opening state: wait=%v drawn=%v target=%v", a.panelAwaitHandle, a.panelW, target)
	}
	for _, px := range []float32{80, 150, 220} {
		tt.Move(px, y)
		tt.Frame()
		tt.Advance(200 * time.Millisecond)
		if a.panelWidth != target || !a.panelAwaitHandle {
			t.Fatalf("pointer %v resized before catch: width=%v waiting=%v", px, a.panelWidth, a.panelAwaitHandle)
		}
	}
	tt.Move(target+6, y)
	tt.Frame()
	tt.Frame()
	if a.panelAwaitHandle || a.panelWidth != target {
		t.Fatalf("catch jumped width: %v waiting=%v", a.panelWidth, a.panelAwaitHandle)
	}
	tt.Move(target+26, y)
	tt.Frame()
	tt.Frame()
	if a.panelWidth != target+20 {
		t.Fatalf("did not resume resize: %v", a.panelWidth)
	}
	tt.Release(target+26, y)
	tt.Frame()
	if a.panelAwaitHandle {
		t.Fatal("release retained latch")
	}
}

func TestEdgePullReleaseClearsHandleWait(t *testing.T) {
	a := New()
	a.flat = true
	a.panelOpen = false
	a.panelW = 0
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	r, _ := tt.Find("调整面板宽度")
	y := r.Y + 40
	tt.Press(6, y)
	tt.Frame()
	tt.Move(46, y)
	tt.Frame()
	tt.Advance(200 * time.Millisecond)
	tt.Release(46, y)
	tt.Frame()
	tt.Frame()
	if a.panelAwaitHandle {
		t.Fatal("release retained wait")
	}
	r, _ = tt.Find("调整面板宽度")
	target := a.panelWidth
	tt.Press(r.X+6, y)
	tt.Frame()
	tt.Move(r.X+26, y)
	tt.Frame()
	tt.Frame()
	if a.panelWidth != target+20 {
		t.Fatalf("fresh drag blocked: %v", a.panelWidth)
	}
}
