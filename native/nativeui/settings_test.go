package nativeui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSettingsDialog(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.openSettings()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	for _, want := range []string{"设置", "主题", "排版", "浅色", "深色"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q; texts = %q", want, tt.Texts())
		}
	}
	if err := tt.Click("排版"); err != nil {
		t.Fatalf("open typography: %v", err)
	}
	tt.Frame()
	if !tt.HasText("字号") || !tt.HasText("行高") {
		t.Fatalf("typography page missing; texts = %q", tt.Texts())
	}
	if err := tt.Click("主题"); err != nil {
		t.Fatalf("back to theme: %v", err)
	}
	tt.Frame()
	if err := tt.Click("深色"); err != nil {
		t.Fatalf("switch theme: %v", err)
	}
	if a.themeMode != 1 {
		t.Fatalf("themeMode = %d, want 1", a.themeMode)
	}
}

func TestAISettingClickPersistsAcrossAppRestart(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", configDir)
	a := New()
	a.openSettings()
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
	if got := loadNativePreferences(); got.AIEnabled {
		t.Fatalf("persisted AI enabled = true after disabling")
	}
	if err := tt.Click("AI 助手开关"); err != nil {
		t.Fatalf("enable AI again: %v", err)
	}
	tt.Frame()
	if !a.aiEnabled {
		t.Fatal("AI switch click did not re-enable the feature")
	}
	restarted := New()
	restarted.applyNativePreferences(loadNativePreferences())
	if !restarted.aiEnabled {
		t.Fatal("AI setting did not survive recreating the app")
	}
	if restarted.themeMode != themeSystem {
		t.Fatalf("missing saved theme must default to system, got %d", restarted.themeMode)
	}
}

func TestThemePreferenceRoundTripsAndDefaultsToSystem(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.applyNativePreferences(loadNativePreferences())
	if a.themeMode != themeSystem {
		t.Fatalf("missing preference theme = %d, want system", a.themeMode)
	}
	a.setThemeMode(themeDark)
	if got := loadNativePreferences(); got.Theme != "dark" {
		t.Fatalf("saved theme = %q, want dark", got.Theme)
	}
	restarted := New()
	restarted.applyNativePreferences(loadNativePreferences())
	if restarted.themeMode != themeDark {
		t.Fatalf("reloaded theme = %d, want dark", restarted.themeMode)
	}
}

func TestSettingsPersistenceFailureIsVisible(t *testing.T) {
	root := t.TempDir()
	badConfigDir := filepath.Join(root, "file")
	if err := os.WriteFile(badConfigDir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MUSE_CONFIG_DIR", badConfigDir)
	a := New()
	a.setAIEnabled(true)
	if !strings.Contains(a.status, "保存设置失败") {
		t.Fatalf("status = %q, want visible preference write error", a.status)
	}
}
