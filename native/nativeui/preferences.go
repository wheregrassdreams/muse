package nativeui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	themeLight = iota
	themeDark
	themeSystem
)

type nativePreferences struct {
	Theme     string `json:"theme,omitempty"`
	AIEnabled bool   `json:"aiEnabled,omitempty"`
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
	p := nativePreferences{AIEnabled: a.aiEnabled}
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

func (a *App) setThemeMode(mode int) {
	if mode < themeLight || mode > themeSystem || a.themeMode == mode {
		return
	}
	a.themeMode = mode
	a.themeApplied = -1
	if err := a.saveNativePreferences(); err != nil {
		a.notice("保存设置失败：" + err.Error())
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
		a.notice("保存设置失败：" + err.Error())
	}
	a.invalidate()
}
