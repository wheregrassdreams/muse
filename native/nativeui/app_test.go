package nativeui

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	workspaceapp "muse/application/workspace"
	settingsui "muse/nativeui/settings"
	"muse/platform/filesystem/recentstore"
)

func TestNativeShellRendersMarkdown(t *testing.T) {
	a := New()
	if err := a.noteEditor().ReplaceUser("# 标题\n\n正文 **加粗**。\n\n- 一\n- 二\n"); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	headings := a.noteEditor().NavigationCommands().Headings()
	if len(headings) != 1 || headings[0].Text != "标题" {
		t.Fatalf("headings = %+v", headings)
	}
	// The document is drawn by the editor: each of its blocks has a place
	// on the surface.
	for _, want := range []string{"标题", "正文", "加粗", "一", "二"} {
		off := strings.Index(a.documentSource(), want)
		if _, _, ok := a.noteEditor().NavigationCommands().WindowPoint(off); !ok {
			t.Fatalf("%q has no place in the editor", want)
		}
	}
}

func TestVimEscapeReachesEditorWhenNoHostedModalIsOpen(t *testing.T) {
	a := New()
	if err := a.noteEditor().ReplaceUser("# Start\n"); err != nil {
		t.Fatal(err)
	}
	a.noteEditor().ModalCommands().SetKeymap("vim")
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	a.noteEditor().FocusCommands().Focus()
	tt.Frame()
	tt.Type("i")
	tt.Frame()
	if got := a.noteEditor().ModalCommands().ModeLabel(); !strings.Contains(strings.ToLower(got), "insert") {
		t.Fatalf("Vim did not enter insert mode: %q", got)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if got := a.noteEditor().ModalCommands().ModeLabel(); !strings.Contains(strings.ToLower(got), "normal") {
		t.Fatalf("Escape was swallowed by the workbench: mode=%q", got)
	}
}

func TestBlankLineSpaceSurvivesRealPaneNavigation(t *testing.T) {
	a := New()
	if err := a.noteEditor().ReplaceUser("alpha\n\nbeta"); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	note := a.noteEditor()
	note.FocusCommands().Focus()
	tt.Frame()
	nav := note.NavigationCommands()
	nav.Select(len("alpha\n"), len("alpha\n"))
	tt.Type(" ")
	tt.Frame()
	want := "alpha\n \nbeta"
	if got := note.Source(); got != want {
		t.Fatalf("source after typing on blank line = %q, want %q", got, want)
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if got := note.Source(); got != want {
		t.Fatalf("source after moving away and back = %q, want %q", got, want)
	}
	if _, _, ok := nav.WindowPoint(len("alpha\n")); !ok {
		t.Fatal("blank-line source position lost its display geometry after navigation")
	}
}

func TestWorkspaceSearchModalFocusAndEscape(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1200, 800)
	a.openWorkspaceSearch()
	tt.Frame()
	if !a.workspaceSearch.IsOpen() {
		t.Fatal("workspace search modal did not open")
	}
	tt.Type("needle")
	tt.Frame()
	if a.workspaceSearch.Query() != "needle" {
		t.Fatalf("autofocused search input got %q", a.workspaceSearch.Query())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.workspaceSearch.IsOpen() {
		t.Fatal("Escape did not close workspace search")
	}
}

func TestWorkspaceSearchClosesOnBackdropClick(t *testing.T) {
	a := New()
	a.openWorkspaceSearch()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()

	// Clicking inside the dialog keeps it open and continues to target the
	// search input. A press on the dimmed area closes it.
	tt.ClickAt(400, 120)
	tt.Frame()
	if !a.workspaceSearch.IsOpen() {
		t.Fatal("clicking inside the search dialog closed it")
	}
	tt.ClickAt(20, 20)
	tt.Frame()
	if a.workspaceSearch.IsOpen() {
		t.Fatal("clicking outside the search dialog did not close it")
	}
}

func TestOpenDialogsRenderIndependentlyOfKeyboardOwnership(t *testing.T) {
	a := New()
	a.openSettings()
	a.openWorkspaceSearch()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("设置") || !tt.HasText("搜索笔记库") {
		t.Fatalf("one open dialog was hidden by another: %q", tt.Texts())
	}
	if !a.settingsPanel.IsOpen() || !a.workspaceSearch.IsOpen() {
		t.Fatalf("rendering changed dialog state: settings=%v search=%v", a.settingsPanel.IsOpen(), a.workspaceSearch.IsOpen())
	}
}

func TestCmdOOpensWorkspaceQuickSearch(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyO)
	tt.Frame()
	if !a.workspaceSearch.IsOpen() || a.quickOpenOpen() {
		t.Fatalf("Cmd+O should open workspace quick search: workspaceSearch=%v quickOpen=%v", a.workspaceSearch.IsOpen(), a.quickOpenOpen())
	}
	if !tt.HasText("搜索笔记库") {
		t.Fatalf("Cmd+O opened the wrong panel: %q", tt.Texts())
	}
}

func TestWorkspaceSearchEmacsNavigationAndVaultRelativePaths(t *testing.T) {
	config := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", config)
	root := t.TempDir()
	nested := filepath.Join(root, "Projects", "Atlas")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "Home.md")
	rendering := filepath.Join(nested, "Rendering Notes.md")
	for path, content := range map[string]string{home: "# Home\n", rendering: "# Rendering Notes\n"} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := New()
	a.SetDependencies(Dependencies{RecentItems: workspaceapp.NewRecentItems(recentstore.New(filepath.Join(config, "recent.json")))})
	a.setWorkspace(root)
	if err := a.recentItems.Add(rendering, false); err != nil {
		t.Fatal(err)
	}
	if err := a.recentItems.Add(home, false); err != nil {
		t.Fatal(err)
	}
	a.openWorkspaceSearch()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	fadeShot(t, tt.Image(), "workspace-search-relative-paths")

	text := strings.Join(tt.Texts(), "\n")
	if strings.Contains(text, root) {
		t.Fatalf("recent path included the vault root: %q", text)
	}
	if !strings.Contains(text, "Projects/Atlas/Rendering Notes.md") {
		t.Fatalf("nested recent path lost its folder hierarchy: %q", text)
	}
	if a.workspaceSearch.Selection() != 0 {
		t.Fatalf("initial selection = %d, want 0", a.workspaceSearch.Selection())
	}
	tt.Key(ui.Ctrl, ui.KeyN)
	tt.Frame()
	if a.workspaceSearch.Selection() != 1 {
		t.Fatalf("Ctrl+N selection = %d, want 1", a.workspaceSearch.Selection())
	}
	tt.Key(ui.Ctrl, ui.KeyP)
	tt.Frame()
	if a.workspaceSearch.Selection() != 0 {
		t.Fatalf("Ctrl+P selection = %d, want 0", a.workspaceSearch.Selection())
	}
}

func TestSidebarMagnifierOpensWorkspaceSearch(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索笔记库"); err != nil {
		t.Fatalf("click sidebar search icon: %v", err)
	}
	tt.Frame()
	if !a.workspaceSearch.IsOpen() || !tt.HasText("搜索笔记库") {
		t.Fatal("sidebar magnifier did not open the workspace search modal")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.workspaceSearch.IsOpen() {
		t.Fatal("Escape did not dismiss the modal opened from the sidebar")
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

	a := newFileTreeTestApp(t, dir)
	a.setWorkspace(dir)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("one.md") {
		t.Fatalf("file tree missing one.md; texts = %q", tt.Texts())
	}
	clickTreeRow(t, tt, "one.md")
	if a.documentPath() != filepath.Join(dir, "one.md") {
		t.Fatalf("opened %q, want one.md", a.documentPath())
	}
	if a.documentSource() != "# One\n\nFirst.\n" {
		t.Fatalf("source = %q", a.documentSource())
	}
}

// TestSidebarModulesInsetFromWindowEdge: the views this sidebar can show sit
// on a quiet row of its own, inset from the window's edge, with the settings
// of the app at the sidebar's foot.
func TestSidebarModulesInsetFromWindowEdge(t *testing.T) {
	a := New()
	a.settingsPanel.Dispatch(settingsui.Action{Kind: settingsui.AssistantEnabledChanged, Enabled: true})
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	for _, label := range []string{"搜索笔记库", "AI 助手", "新建文档"} {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("module %q missing", label)
		}
		if r.X < 7 {
			t.Fatalf("%q starts at x=%.1f, want it inset from the window's edge", label, r.X)
		}
		if right := r.X + r.W; right > a.panelWidth {
			t.Fatalf("%q ends at x=%.1f, past the sidebar's width %.0f", label, right, a.panelWidth)
		}
	}
	if _, ok := tt.Find("设置"); !ok {
		t.Fatal("no settings control at the sidebar's foot")
	}
}

func TestSidebarDefaultHidesAIAndFollowsSystemInHeadless(t *testing.T) {
	captureDir := os.Getenv("MUSE_SIDEBAR_CAPTURE_DIR")
	for _, tc := range []struct {
		name string
		dark bool
	}{{"light", false}, {"dark", true}} {
		t.Run(tc.name, func(t *testing.T) {
			a := New()
			tt := ui.NewTesterWith(a.View, 1200, 800, ui.TesterOptions{Dark: tc.dark})
			tt.Frame()
			if tt.HasText("AI 助手") {
				t.Fatal("AI is visible while disabled")
			}
			iconRect, ok := tt.Find("设置")
			if !ok {
				t.Fatal("sidebar settings icon is missing")
			}
			img := tt.Image()
			background := img.At(100, 400)
			if got := colorDistance(background, color.NRGBA{R: 255, G: 255, B: 255, A: 255}); !tc.dark && got > 20 {
				t.Fatalf("light sidebar background differs from the light theme: distance %.1f", got)
			}
			if got := colorDistance(background, color.NRGBA{R: 35, G: 35, B: 40, A: 255}); tc.dark && got > 25 {
				t.Fatalf("dark sidebar background differs from the dark theme: distance %.1f", got)
			}
			iconPixels := 0
			for y := int(iconRect.Y); y < int(iconRect.Y+iconRect.H); y++ {
				for x := int(iconRect.X); x < int(iconRect.X+iconRect.W); x++ {
					if colorDistance(img.At(x, y), background) > 18 {
						iconPixels++
					}
				}
			}
			if iconPixels < 3 {
				t.Fatalf("sidebar settings icon has only %d pixels distinct from its background", iconPixels)
			}
			if captureDir != "" {
				if err := os.MkdirAll(captureDir, 0o700); err != nil {
					t.Fatal(err)
				}
				f, err := os.Create(filepath.Join(captureDir, tc.name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(f, img); err != nil {
					f.Close()
					t.Fatal(err)
				}
				if err := f.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func colorDistance(a, b color.Color) float64 {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	dr, dg, db := float64(int64(ar)-int64(br))/257, float64(int64(ag)-int64(bg))/257, float64(int64(ab)-int64(bb))/257
	return (dr*dr + dg*dg + db*db) / 3
}

func TestRailSelectsModulesAndHeaderCollapses(t *testing.T) {
	a := New()
	a.settingsPanel.Dispatch(settingsui.Action{Kind: settingsui.AssistantEnabledChanged, Enabled: true})
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !a.panelOpen {
		t.Fatal("panel should start open")
	}
	if err := tt.Click("AI 助手"); err != nil {
		t.Fatalf("click AI: %v", err)
	}
	tt.Frame()
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
	if err := tt.Click("侧栏"); err != nil {
		t.Fatalf("reopen through the sidebar toggle: %v", err)
	}
	tt.Frame()
	if !a.panelOpen {
		t.Fatal("the sidebar toggle did not reopen the panel")
	}
}

func TestDarkThemeRenders(t *testing.T) {
	a := New()
	if err := a.noteEditor().ReplaceUser("# 标题\n\n正文。\n\n```go\nx := 1\n```\n"); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.View, 1200, 800)
	tt.SetDark(true)
	tt.Frame()
	tt.Frame()
	if _, _, ok := a.noteEditor().NavigationCommands().WindowPoint(strings.Index(a.documentSource(), "正文")); !ok {
		t.Fatal("the document is not laid out in the dark theme")
	}
}
