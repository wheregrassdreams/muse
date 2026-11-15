package nativeui

import (
	"encoding/json"
	"muse/appearance"
	"muse/locales"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

const (
	themeLight = iota
	themeDark
	themeSystem
)

type nativePreferences struct {
	Theme            string `json:"theme,omitempty"`
	Accent           string `json:"accent,omitempty"`
	AIEnabled        bool   `json:"aiEnabled,omitempty"`
	SmoothPageJumps  bool   `json:"smoothPageJumps,omitempty"`
	Locale           string `json:"locale,omitempty"`
	LocaleConfigured bool   `json:"localeConfigured,omitempty"`
}

func nativePreferencesPath() string {
	if dir := os.Getenv("MUSE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Muse", "settings.json")
}

func loadNativePreferences() nativePreferences {
	var p nativePreferences
	b, err := os.ReadFile(nativePreferencesPath())
	if err == nil {
		_ = json.Unmarshal(b, &p)
	}
	return p
}

func (a *App) applyNativePreferences(p nativePreferences) {
	a.aiEnabled = p.AIEnabled
	a.smoothPageJumps = p.SmoothPageJumps
	a.localeConfigured = p.LocaleConfigured
	a.localeSetting = p.Locale
	if p.LocaleConfigured {
		mygo.App.SetLocale(p.Locale)
	}
	if a.ed != nil {
		a.ed.SetOptions(a.editorOptions())
	}
	a.accentID = appearance.Normalize(p.Accent)
	if mode, ok := themeModeFromPreference(p.Theme); ok {
		a.themeMode = mode
	} else {
		a.themeMode = themeSystem
	}
}

func themeModeFromPreference(value string) (int, bool) {
	switch value {
	case "light":
		return themeLight, true
	case "dark":
		return themeDark, true
	case "system":
		return themeSystem, true
	default:
		return 0, false
	}
}

func (a *App) saveNativePreferences() error {
	p := nativePreferences{
		AIEnabled: a.aiEnabled, SmoothPageJumps: a.smoothPageJumps,
		Accent: appearance.Normalize(a.accentID), Locale: a.localeSetting,
		LocaleConfigured: a.localeConfigured,
	}
	switch a.themeMode {
	case themeDark:
		p.Theme = "dark"
	case themeSystem:
		p.Theme = "system"
	default:
		p.Theme = "light"
	}
	path := nativePreferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func (a *App) setLocale(tag string) {
	if a.localeConfigured && a.localeSetting == tag {
		return
	}
	a.localeConfigured = true
	a.localeSetting = tag
	if err := a.saveNativePreferences(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	mygo.App.SetLocale(tag)
}

func (a *App) setThemeMode(mode int) {
	if mode < themeLight || mode > themeSystem || a.themeMode == mode {
		return
	}
	a.themeMode = mode
	a.themeApplied = -1
	if err := a.saveNativePreferences(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setAIEnabled(enabled bool) {
	if a.aiEnabled == enabled {
		return
	}
	a.aiEnabled = enabled
	if !enabled && a.panel == panelAI {
		a.showPanel(panelFiles)
	}
	if err := a.saveNativePreferences(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setSmoothPageJumps(enabled bool) {
	if a.smoothPageJumps == enabled {
		return
	}
	a.smoothPageJumps = enabled
	a.ed.SetOptions(a.editorOptions())
	if err := a.saveNativePreferences(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}
