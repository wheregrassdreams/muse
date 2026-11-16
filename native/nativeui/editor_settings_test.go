package nativeui

import (
	"github.com/egoist/mygo/ui"
	"muse/locales"
	"os"
	"testing"
	"time"
)

func TestEditorKeymapPersists(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	if a.activeNote.ModalCommands().Keymap() != "default" {
		t.Fatal(a.activeNote.ModalCommands().Keymap())
	}
	a.setEditorKeymap("vim")
	b := New()
	if b.activeNote.ModalCommands().Keymap() != "vim" {
		t.Fatalf("restored %q", b.activeNote.ModalCommands().Keymap())
	}
	if err := os.WriteFile(editorConfigPath(), []byte(`{"keymap":"unknown"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := New().activeNote.ModalCommands().Keymap(); got != "default" {
		t.Fatal(got)
	}
}

func TestEditorKeymapSettingsUI(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.openSettings()
	a.settingsTab = 1
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if err := tt.Click("按键模式：Vim"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.activeNote.ModalCommands().Keymap() != "vim" {
		t.Fatal(a.activeNote.ModalCommands().Keymap())
	}
	if err := tt.Click("按键模式：默认"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.activeNote.ModalCommands().Keymap() != "default" {
		t.Fatal(a.activeNote.ModalCommands().Keymap())
	}
}

func TestSourceRevealPreferenceIsInformational(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.openSettings()
	a.settingsTab = 1
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("源码展开偏好") || !tt.HasText("当前策略暂不支持自定义") {
		t.Fatal("settings page does not explain the current source reveal policy")
	}
	if _, err := os.Stat(editorConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("showing the informational row wrote editor settings: %v", err)
	}
}

func TestSmartTypographyDefaultsAndReload(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		want         bool
	}{
		{name: "missing file", want: true},
		{name: "legacy keymap", config: `{"keymap":"vim"}`, want: true},
		{name: "enabled", config: `{"keymap":"vim","smartTypography":true}`, want: true},
		{name: "disabled", config: `{"keymap":"vim","smartTypography":false}`, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
			if tc.config != "" {
				if err := os.WriteFile(editorConfigPath(), []byte(tc.config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			a := New()
			if a.smartTypography != tc.want || a.activeNote.PresentationCommands().Settings().DisableSmartTypography != !tc.want {
				t.Fatalf("smartTypography = %v, editor disabled = %v; want enabled %v", a.smartTypography, a.activeNote.PresentationCommands().Settings().DisableSmartTypography, tc.want)
			}
		})
	}
}

func TestSmartTypographyAndKeymapSaveTogether(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.setEditorKeymap("vim")
	a.setSmartTypography(false)
	b := New()
	if b.smartTypography || b.activeNote.ModalCommands().Keymap() != "vim" {
		t.Fatalf("typography save restored enabled=%v keymap=%q", b.smartTypography, b.activeNote.ModalCommands().Keymap())
	}
	b.setEditorKeymap("helix")
	c := New()
	if c.smartTypography || !c.activeNote.PresentationCommands().Settings().DisableSmartTypography || c.activeNote.ModalCommands().Keymap() != "helix" {
		t.Fatalf("keymap save restored enabled=%v disabled=%v keymap=%q", c.smartTypography, c.activeNote.PresentationCommands().Settings().DisableSmartTypography, c.activeNote.ModalCommands().Keymap())
	}
	c.setSmartTypography(true)
	d := New()
	if !d.smartTypography || d.activeNote.PresentationCommands().Settings().DisableSmartTypography || d.activeNote.ModalCommands().Keymap() != "helix" {
		t.Fatalf("re-enable restored enabled=%v disabled=%v keymap=%q", d.smartTypography, d.activeNote.PresentationCommands().Settings().DisableSmartTypography, d.activeNote.ModalCommands().Keymap())
	}
}

func TestSmartTypographySettingsToggle(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.openSettings()
	a.settingsTab = 0
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("智能排版") {
		t.Fatal("missing smart typography setting")
	}
	for _, enabled := range []bool{false, true} {
		if err := tt.Click("智能排版开关"); err != nil {
			t.Fatal(err)
		}
		// The click updates the active editor without waiting for another frame.
		if a.smartTypography != enabled || a.editorOptions().DisableSmartTypography != !enabled || a.activeNote.PresentationCommands().Settings().DisableSmartTypography != !enabled {
			t.Fatalf("toggle: enabled=%v options disabled=%v editor disabled=%v; want enabled %v", a.smartTypography, a.editorOptions().DisableSmartTypography, a.activeNote.PresentationCommands().Settings().DisableSmartTypography, enabled)
		}
		saved := loadEditorSettings()
		if saved.SmartTypography == nil || *saved.SmartTypography != enabled {
			t.Fatalf("saved smartTypography = %v, want %v", saved.SmartTypography, enabled)
		}
		tt.Frame()
	}
}

func TestVimVirtualImageRowsDefaultOffAndPersist(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	if a.vimVirtualImageRows || a.activeNote.PresentationCommands().Settings().VimVirtualImageRows {
		t.Fatal("Vim virtual image rows should default off")
	}
	a.setVimVirtualImageRows(true)
	b := New()
	if !b.vimVirtualImageRows || !b.activeNote.PresentationCommands().Settings().VimVirtualImageRows {
		t.Fatal("Vim virtual image rows were not restored from settings")
	}
	b.setVimVirtualImageRows(false)
	if c := New(); c.vimVirtualImageRows || c.activeNote.PresentationCommands().Settings().VimVirtualImageRows {
		t.Fatal("disabled Vim virtual image rows were not restored")
	}
}

func TestHeadingMarkerRevealDefaultsOffAndPersists(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	if a.headingMarkerReveal || a.activeNote.PresentationCommands().Settings().HeadingMarkerReveal {
		t.Fatal("heading marker reveal preference should default off")
	}
	a.setHeadingMarkerReveal(true)
	b := New()
	if !b.headingMarkerReveal || !b.activeNote.PresentationCommands().Settings().HeadingMarkerReveal {
		t.Fatal("heading marker reveal preference was not restored")
	}
	b.setHeadingMarkerReveal(false)
	if c := New(); c.headingMarkerReveal || c.activeNote.PresentationCommands().Settings().HeadingMarkerReveal {
		t.Fatal("disabled heading marker reveal preference was not restored")
	}
}

func TestHeadingMarkerRevealSettingsToggleThroughEditorPage(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.openSettings()
	a.settingsTab = 1
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if err := tt.Click("标题标记按空格展开开关"); err != nil {
		t.Fatal(err)
	}
	if !a.headingMarkerReveal || !a.activeNote.PresentationCommands().Settings().HeadingMarkerReveal {
		t.Fatal("settings toggle did not update editor heading reveal behavior")
	}
	if saved := loadEditorSettings(); saved.HeadingMarkerReveal == nil || !*saved.HeadingMarkerReveal {
		t.Fatal("settings toggle did not persist heading marker reveal behavior")
	}
}

func TestVimVirtualImageRowsSettingsToggle(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.openSettings()
	a.settingsTab = 1
	a.setEditorKeymap("vim")
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	for _, enabled := range []bool{true, false} {
		if err := tt.Click("Vim 图片虚拟行开关"); err != nil {
			t.Fatal(err)
		}
		if a.vimVirtualImageRows != enabled || a.activeNote.PresentationCommands().Settings().VimVirtualImageRows != enabled {
			t.Fatalf("toggle enabled=%v app=%v editor=%v", enabled, a.vimVirtualImageRows, a.activeNote.PresentationCommands().Settings().VimVirtualImageRows)
		}
		saved := loadEditorSettings()
		if saved.VimVirtualImageRows == nil || *saved.VimVirtualImageRows != enabled {
			t.Fatalf("saved vimVirtualImageRows = %v, want %v", saved.VimVirtualImageRows, enabled)
		}
		tt.Frame()
	}
}

func TestCursorSettingsDefaultOffAndPersist(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	if a.cursorFade || a.smoothCursor || a.hideMouseCursor || a.mouseCursorHidden || a.editorOptions().CursorFade || a.editorOptions().SmoothCursor {
		t.Fatal("cursor effects should default off")
	}
	a.setCursorFade(true)
	a.setSmoothCursor(true)
	a.setHideMouseCursor(true)
	b := New()
	if !b.cursorFade || !b.smoothCursor || !b.hideMouseCursor || !b.editorOptions().CursorFade || !b.editorOptions().SmoothCursor {
		t.Fatal("cursor settings were not restored from settings")
	}
	b.setCursorFade(false)
	b.setSmoothCursor(false)
	b.setHideMouseCursor(false)
	c := New()
	if c.cursorFade || c.smoothCursor || c.hideMouseCursor || c.editorOptions().CursorFade || c.editorOptions().SmoothCursor {
		t.Fatal("disabled cursor settings were not restored")
	}
}

func TestSearchMatchPulseSettingDefaultAndPersist(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	if a.searchMatchPulse || a.editorOptions().SearchMatchPulse || a.activeNote.PresentationCommands().Settings().SearchMatchPulse {
		t.Fatal("search match pulse should default off")
	}
	a.setSearchMatchPulse(true)
	b := New()
	if !b.searchMatchPulse || !b.editorOptions().SearchMatchPulse || !b.activeNote.PresentationCommands().Settings().SearchMatchPulse {
		t.Fatal("search match pulse was not restored from settings")
	}
	if saved := loadEditorSettings(); saved.SearchMatchPulse == nil || !*saved.SearchMatchPulse {
		t.Fatal("search match pulse was not saved")
	}
	b.setSearchMatchPulse(false)
	if c := New(); c.searchMatchPulse || c.activeNote.PresentationCommands().Settings().SearchMatchPulse {
		t.Fatal("disabled search match pulse was not restored")
	}
}

func TestSearchMatchPulseSettingToggleThroughEditorPage(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.openSettings()
	a.settingsTab = 1
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索命中轻微动效开关"); err != nil {
		t.Fatal(err)
	}
	if !a.searchMatchPulse || !a.activeNote.PresentationCommands().Settings().SearchMatchPulse {
		t.Fatal("settings toggle did not enable search match pulse")
	}
	if saved := loadEditorSettings(); saved.SearchMatchPulse == nil || !*saved.SearchMatchPulse {
		t.Fatal("settings toggle did not persist search match pulse")
	}
}

func TestVimYankFlashSettingDefaultAndPersist(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	if a.vimYankFlash || a.editorOptions().VimYankFlash || a.activeNote.PresentationCommands().Settings().VimYankFlash {
		t.Fatal("Vim yank flash should default off")
	}
	a.setEditorKeymap("vim")
	a.setVimYankFlash(true)
	b := New()
	if !b.vimYankFlash || !b.editorOptions().VimYankFlash || !b.activeNote.PresentationCommands().Settings().VimYankFlash {
		t.Fatal("Vim yank flash was not restored from settings")
	}
	if saved := loadEditorSettings(); saved.VimYankFlash == nil || !*saved.VimYankFlash {
		t.Fatal("Vim yank flash was not saved")
	}
	b.setVimYankFlash(false)
	if c := New(); c.vimYankFlash || c.activeNote.PresentationCommands().Settings().VimYankFlash {
		t.Fatal("disabled Vim yank flash was not restored")
	}
}

func TestVimYankFlashSettingsToggleAndVisibility(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.openSettings()
	a.settingsTab = 1
	standard := ui.NewTester(a.View, 1200, 800)
	standard.Frame()
	if standard.HasText("Vim 复制选区闪显") {
		t.Fatal("Vim-only yank setting is visible in default keymap")
	}
	a.setEditorKeymap("vim")
	a.settingsScroll.Y = 1000
	standard.Frame()
	if err := standard.Click("Vim 复制选区闪显开关"); err != nil {
		t.Fatal(err)
	}
	if !a.vimYankFlash || !a.activeNote.PresentationCommands().Settings().VimYankFlash {
		t.Fatal("Vim yank flash toggle did not update app and editor")
	}
}

func TestCursorSettingsToggleThroughEditorPage(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.setEditorKeymap("vim")
	a.openSettings()
	a.settingsTab = 1
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	for _, tc := range []struct {
		label string
		set   func() bool
		opt   func() bool
	}{
		{"光标淡入淡出开关", func() bool { return a.cursorFade }, func() bool { return a.activeNote.PresentationCommands().Settings().CursorFade }},
		{"平滑光标开关", func() bool { return a.smoothCursor }, func() bool { return a.activeNote.PresentationCommands().Settings().SmoothCursor }},
		{locales.SettingsHideMouseCursor(), func() bool { return a.hideMouseCursor }, func() bool { return a.hideMouseCursor }},
	} {
		if err := tt.Click(tc.label); err != nil {
			t.Fatal(err)
		}
		if !tc.set() || !tc.opt() {
			t.Fatalf("%s did not update app and editor options", tc.label)
		}
		saved := loadEditorSettings()
		if (tc.label == "光标淡入淡出开关" && (saved.CursorFade == nil || !*saved.CursorFade)) ||
			(tc.label == "平滑光标开关" && (saved.SmoothCursor == nil || !*saved.SmoothCursor)) {
			t.Fatalf("%s was not persisted", tc.label)
		}
		if tc.label == locales.SettingsHideMouseCursor() && (saved.HideMouseCursor == nil || !*saved.HideMouseCursor) {
			t.Fatalf("%s was not persisted", tc.label)
		}
		tt.Frame()
	}
}

func TestHideMouseCursorAfterTypingAndVimInputUntilIdle(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.setHideMouseCursor(true)
	tt := ui.NewTester(a.View, 1200, 800)
	a.activeNote.FocusCommands().Focus()
	tt.Frame()
	tt.Move(700, 400)
	tt.Type("x")
	if !a.mouseCursorHidden {
		t.Fatal("typing did not hide the mouse pointer")
	}
	if got := tt.Cursor(); got != ui.CursorNone {
		t.Fatalf("native pointer cursor = %v after typing, want CursorNone", got)
	}
	tt.Advance(2 * time.Second)
	if a.mouseCursorHidden || tt.Cursor() == ui.CursorNone {
		t.Fatal("pointer was not restored after typing stopped")
	}
	a.setEditorKeymap("vim")
	a.activeNote.FocusCommands().Focus()
	tt.Frame()
	tt.Key(0, ui.KeyJ)
	if !a.mouseCursorHidden {
		t.Fatal("Vim key operation did not hide the mouse pointer")
	}
	if got := tt.Cursor(); got != ui.CursorNone {
		t.Fatalf("native pointer cursor = %v after Vim input, want CursorNone", got)
	}
	tt.Advance(2 * time.Second)
	if a.mouseCursorHidden || tt.Cursor() == ui.CursorNone {
		t.Fatal("pointer was not restored after Vim input stopped")
	}
}

func TestEditorSettingsHideModeSpecificRows(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.openSettings()
	a.settingsTab = 1
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if tt.HasText("Vim 图片虚拟行") || tt.HasText("平滑光标") {
		t.Fatal("modal-only settings should be hidden in default keymap")
	}
	a.setEditorKeymap("helix")
	tt.Frame()
	if !tt.HasText("平滑光标") || tt.HasText("Vim 图片虚拟行") {
		t.Fatal("Helix should show smooth cursor only")
	}
	a.setEditorKeymap("vim")
	tt.Frame()
	if !tt.HasText("平滑光标") || !tt.HasText("Vim 图片虚拟行") {
		t.Fatal("Vim should show Vim and modal cursor settings")
	}
}
