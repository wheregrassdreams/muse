package nativeui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestNativeShellRendersMarkdown(t *testing.T) {
	a := New()
	a.loadDoc("# 标题\n\n正文 **加粗**。\n\n- 一\n- 二\n")
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	if len(a.headings) != 1 || a.headings[0].text != "标题" {
		t.Fatalf("headings = %+v", a.headings)
	}
	// The document is drawn by the editor: each of its blocks has a place
	// on the surface.
	for _, want := range []string{"标题", "正文", "加粗", "一", "二"} {
		off := strings.Index(a.source, want)
		if _, _, ok := a.ed.WindowPoint(off); !ok {
			t.Fatalf("%q has no place in the editor", want)
		}
	}
}

func TestOpenWorkspaceAndFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one.md"), []byte("# One\n\nFirst.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "two.md"), []byte("# Two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := New()
	a.setWorkspace(dir)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("one.md") {
		t.Fatalf("file tree missing one.md; texts = %q", tt.Texts())
	}
	if err := tt.Click("one.md"); err != nil {
		t.Fatalf("click file: %v", err)
	}
	if a.path != filepath.Join(dir, "one.md") {
		t.Fatalf("opened %q, want one.md", a.path)
	}
	if a.source != "# One\n\nFirst.\n" {
		t.Fatalf("source = %q", a.source)
	}
}

func TestRailButtonsInsetFromWindowEdge(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	modules := []string{"文件", "大纲", "搜索", "AI 助手"}
	prevBottom := float32(-1)
	for i, label := range append(modules, "设置") {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("rail button %q missing", label)
		}
		if r.X < railInset-0.5 {
			t.Fatalf("%q starts at x=%.1f, want at least %.1f", label, r.X, railInset)
		}
		if right := r.X + r.W; right > railWidth-railInset+0.5 {
			t.Fatalf("%q ends at x=%.1f, want at most %.1f", label, right, railWidth-railInset)
		}
		if i < len(modules) {
			if prevBottom >= 0 && r.Y-prevBottom < 3.5 {
				t.Fatalf("%q sits %.1fpx below the icon above, want a 4px gap", label, r.Y-prevBottom)
			}
			prevBottom = r.Y + r.H
		}
	}
}

func TestRailSelectsModulesAndHeaderCollapses(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !a.panelOpen {
		t.Fatal("panel should start open")
	}
	if err := tt.Click("AI 助手"); err != nil {
		t.Fatalf("click AI: %v", err)
	}
	tt.Frame()
	if a.panel != panelAI || !a.panelOpen {
		t.Fatalf("panel = %v, open = %v", a.panel, a.panelOpen)
	}
	// The active module is a view switch now: a second click (or a
	// double-click) must not hide the panel.
	tt.Advance(time.Second)
	if err := tt.Click("AI 助手"); err != nil {
		t.Fatalf("click AI again: %v", err)
	}
	tt.Frame()
	if !a.panelOpen {
		t.Fatal("clicking the active rail icon collapsed the panel")
	}
	// The one sidebar toggle folds the panel away.
	tt.Advance(time.Second)
	if err := tt.Click("侧栏"); err != nil {
		t.Fatalf("collapse: %v", err)
	}
	tt.Frame()
	if a.panelOpen {
		t.Fatal("the header icon did not collapse the panel")
	}
	tt.Advance(time.Second)
	if err := tt.Click("文件"); err != nil {
		t.Fatalf("reopen files: %v", err)
	}
	tt.Frame()
	if !a.panelOpen || a.panel != panelFiles {
		t.Fatalf("reopen: panel = %v, open = %v", a.panel, a.panelOpen)
	}
}

// TestThemeSwitchForcesTheAppAppearance checks that the app's own acrylic
// and title bar follow the theme it draws, not the desktop's appearance:
// the material behind the rail and the panel is the system's, and would
// stay light in a dark app otherwise. The first frame asks for the theme
// in use, an unchanged frame asks for nothing, and every switch asks for
// the new one.
func TestThemeSwitchForcesTheAppAppearance(t *testing.T) {
	prev := setAppAppearance
	var asked []mygo.ThemeSource
	setAppAppearance = func(s mygo.ThemeSource) { asked = append(asked, s) }
	t.Cleanup(func() { setAppAppearance = prev })

	a := New()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	a.themeMode = 1
	tt.Frame()
	a.themeMode = 0
	tt.Frame()
	want := []mygo.ThemeSource{mygo.ThemeLight, mygo.ThemeDark, mygo.ThemeLight}
	if !slices.Equal(asked, want) {
		t.Fatalf("appearance requests = %v, want %v", asked, want)
	}
}

func TestDarkThemeRenders(t *testing.T) {
	a := New()
	a.loadDoc("# 标题\n\n正文。\n\n```go\nx := 1\n```\n")
	a.themeMode = 1
	tt := ui.NewTester(a.View, 1200, 800)
	tt.SetDark(true)
	tt.Frame()
	tt.Frame()
	if !a.dark {
		t.Fatal("the dark theme is not on")
	}
	if _, _, ok := a.ed.WindowPoint(strings.Index(a.source, "正文")); !ok {
		t.Fatal("the document is not laid out in the dark theme")
	}
}

func TestPanelCollapseAnimatesInsteadOfSnapping(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if a.panelW != a.panelWidth {
		t.Fatalf("panel starts at %.1f, want %.1f", a.panelW, a.panelWidth)
	}
	a.collapsePanel()
	tt.Frame()
	tt.Advance(30 * time.Millisecond)
	if a.panelW <= 0 || a.panelW >= a.panelWidth {
		t.Fatalf("the panel did not slide: width %.1f", a.panelW)
	}
	tt.Advance(time.Second)
	if a.panelW != 0 {
		t.Fatalf("panel width = %.1f after the slide, want 0", a.panelW)
	}
	a.showPanel(panelOutline)
	tt.Frame() // the frame that retargets the animation
	tt.Advance(30 * time.Millisecond)
	if a.panelW <= 0 {
		t.Fatal("reopening did not start the slide")
	}
}
