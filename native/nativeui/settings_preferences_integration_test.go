package nativeui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	settingsui "muse/nativeui/settings"
)

func TestSettingsDialog(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := newConfigTestApp()
	a.openSettings()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	for _, want := range []string{"设置", "主题", "排版", "浅色", "深色", "智能排版", "字号", "行高"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q; texts = %q", want, tt.Texts())
		}
	}
	if tt.HasText("AI 助手") {
		t.Fatal("AI assistant control should be on the AI page, not appearance")
	}
	if err := tt.Click("编辑器"); err != nil {
		t.Fatalf("open editor settings: %v", err)
	}
	tt.Frame()
	for _, want := range []string{"按键模式", "光标淡入淡出", "源码展开偏好"} {
		if !tt.HasText(want) {
			t.Fatalf("editor page missing %q; texts = %q", want, tt.Texts())
		}
	}
	if tt.HasText("智能排版") || tt.HasText("字号") || tt.HasText("行高") {
		t.Fatalf("appearance controls leaked into editor page; texts = %q", tt.Texts())
	}
	if err := tt.Click("外观"); err != nil {
		t.Fatalf("back to appearance: %v", err)
	}
	tt.Frame()
	if err := tt.Click("深色"); err != nil {
		t.Fatalf("switch theme: %v", err)
	}
	if a.themeMode != 1 {
		t.Fatalf("themeMode = %d, want 1", a.themeMode)
	}
	if err := tt.Click("AI"); err != nil {
		t.Fatalf("open AI settings: %v", err)
	}
	tt.Frame()
	for _, want := range []string{"AI 助手", "服务", "接口地址", "API Key", "模型"} {
		if !tt.HasText(want) {
			t.Fatalf("AI settings page missing %q; texts = %q", want, tt.Texts())
		}
	}
}

func TestAISettingClickPersistsAcrossAppRestart(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", configDir)
	a := newConfigTestApp()
	a.openSettings()
	a.settingsPanel.SetPage(settingsui.AssistantPage)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if a.aiEnabled {
		t.Fatal("AI should start disabled")
	}
	if err := tt.Click("AI 助手开关"); err != nil {
		t.Fatalf("enable AI: %v", err)
	}
	tt.Frame()
	if !a.aiEnabled {
		t.Fatal("AI switch click did not enable the feature")
	}
	if err := tt.Click("AI 助手开关"); err != nil {
		t.Fatalf("disable AI: %v", err)
	}
	tt.Frame()
	if a.aiEnabled {
		t.Fatal("AI switch click did not disable the feature")
	}
	if got := a.loadNativePreferences(); got.AIEnabled {
		t.Fatalf("persisted AI enabled = true after disabling")
	}
	if err := tt.Click("AI 助手开关"); err != nil {
		t.Fatalf("enable AI again: %v", err)
	}
	tt.Frame()
	if !a.aiEnabled {
		t.Fatal("AI switch click did not re-enable the feature")
	}
	restarted := newConfigTestApp()
	restarted.applyNativePreferences(restarted.loadNativePreferences())
	if !restarted.aiEnabled {
		t.Fatal("AI setting did not survive recreating the app")
	}
	if restarted.themeMode != themeSystem {
		t.Fatalf("missing saved theme must default to system, got %d", restarted.themeMode)
	}
}

func TestThemePreferenceRoundTripsAndDefaultsToSystem(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := newConfigTestApp()
	a.applyNativePreferences(a.loadNativePreferences())
	if a.themeMode != themeSystem {
		t.Fatalf("missing preference theme = %d, want system", a.themeMode)
	}
	a.setThemeMode(themeDark)
	if got := a.loadNativePreferences(); got.Theme != "dark" {
		t.Fatalf("saved theme = %q, want dark", got.Theme)
	}
	restarted := newConfigTestApp()
	restarted.applyNativePreferences(restarted.loadNativePreferences())
	if restarted.themeMode != themeDark {
		t.Fatalf("reloaded theme = %d, want dark", restarted.themeMode)
	}
}

func TestSmoothPageJumpAppearanceSettingDefaultsOffAndPersists(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := newConfigTestApp()
	a.applyNativePreferences(a.loadNativePreferences())
	if a.smoothPageJumps || a.noteEditor().PresentationCommands().Settings().SmoothPageJumps {
		t.Fatal("smooth page jumps should default off")
	}
	a.openSettings()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("页内跳转平滑滚动") {
		t.Fatal("missing appearance option for smooth in-page navigation")
	}
	tt.Scroll(800, 500, 0, 1000)
	tt.Frame()
	if err := tt.Click("页内跳转平滑滚动开关"); err != nil {
		t.Fatalf("enable smooth page jumps: %v", err)
	}
	if !a.smoothPageJumps || !a.noteEditor().PresentationCommands().Settings().SmoothPageJumps {
		t.Fatal("setting did not update editor navigation options")
	}
	restarted := newConfigTestApp()
	restarted.applyNativePreferences(restarted.loadNativePreferences())
	if !restarted.smoothPageJumps || !restarted.noteEditor().PresentationCommands().Settings().SmoothPageJumps {
		t.Fatal("smooth page jump setting did not persist")
	}
}

func TestSettingsPersistenceFailureIsVisible(t *testing.T) {
	root := t.TempDir()
	badConfigDir := filepath.Join(root, "file")
	if err := os.WriteFile(badConfigDir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MUSE_CONFIG_DIR", badConfigDir)
	a := newConfigTestApp()
	a.setAIEnabled(true)
	if !strings.Contains(a.status, "保存设置失败") {
		t.Fatalf("status = %q, want visible preference write error", a.status)
	}
}
