package nativeui

import (
	"github.com/egoist/mygo/ui"
	appearance "muse/theme"
	"testing"
)

func TestAccentSettingsPersistAndFollowTheme(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := newConfigTestApp()
	a.applyNativePreferences(a.loadNativePreferences())
	if a.accentID != "red" {
		t.Fatalf("default accent=%q", a.accentID)
	}
	a.openSettings()
	var theme ui.Theme
	tt := ui.NewTester(func(c *ui.Context) { a.View(c); theme = *c.Theme() }, 1200, 800)
	tt.Frame()
	if !tt.HasText("重点色") {
		t.Fatal("missing accent palette")
	}
	fadeShot(t, tt.Image(), "accent-settings-red-light")
	if err := tt.Click("重点色：蓝色"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.accentID != "blue" || a.loadNativePreferences().Accent != "blue" {
		t.Fatal("accent click was not persisted")
	}
	if theme.Accent != appearance.From(appearance.Color("blue"), a.dark).Accent {
		t.Fatal("theme did not use selected accent")
	}
	a.setThemeMode(themeDark)
	a.setAIEnabled(true)
	tt.Frame()
	if theme.Accent != appearance.From(appearance.Color("blue"), true).Accent {
		t.Fatal("accent did not adapt to dark theme")
	}
	fadeShot(t, tt.Image(), "accent-settings-blue-dark")
	restarted := newConfigTestApp()
	restarted.applyNativePreferences(restarted.loadNativePreferences())
	if restarted.accentID != "blue" || restarted.themeMode != themeDark || !restarted.aiEnabled {
		t.Fatal("settings overwrote one another")
	}
	restarted.applyNativePreferences(nativePreferences{Accent: "unknown"})
	if restarted.accentID != "red" {
		t.Fatal("invalid saved accent should fall back to red")
	}
}

func TestAccentDocumentThemeScreenshots(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	const src = "# 这是一个标题\n\n一个很长的段落，用来检查换行与滚动：在很长很长的文本里，排版应当把每一行都摆对，光标应当能落在任意两字之间。再加一些标点，括号（圆括号）、【方括号】、「引号」，以及 an occasional English sentence.\n\n## 添加文字样式\n\n你可以为文本添加各种样式，包括：**粗体**、*斜体*、~~删除线~~、[链接](https://example.com)、`行内代码`、==高亮==、#标签，以及公式 $a^2+b^2=c^2$。\n\n> 普通引用。\n> 第二行。\n\n> [!NOTE] 提示\n> callout 的正文。\n\n| 项目 | 内容 |\n| --- | --- |\n| 表格 | 与正文协调 |"
	for _, dark := range []bool{false, true} {
		a := newConfigTestApp()
		a.panelOpen = false
		a.panelW = 0
		a.panelWidth = 0
		name := "bear-style-red-light"
		if dark {
			a.themeMode = themeDark
			name = "bear-style-red-dark"
		} else {
			a.themeMode = themeLight
		}
		a.loadDoc(src)
		tt := ui.NewTester(a.View, 1200, 1000)
		tt.Frame()
		tt.Frame()
		if a.accentID != "red" {
			t.Fatal("default must remain red")
		}
		fadeShot(t, tt.Image(), name)
	}
}
