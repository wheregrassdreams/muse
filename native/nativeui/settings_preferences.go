package nativeui

import (
	applicationconfig "muse/application/config"
	"muse/config"
	"muse/locales"
	appearance "muse/theme"

	"github.com/egoist/mygo"
)

// These values are presentation state used by NativeUI to adapt the system
// surfaces. The persisted preference itself uses config.ThemeMode.
const (
	themeLight = iota
	themeDark
	themeSystem
)

type nativePreferences = config.Preferences

// loadNativePreferences and applyNativePreferences are composition-root
// adapters: the settings feature receives a view model, while NativeUI owns
// persistence and effects on the app, editor, and platform locale.
func (a *App) loadNativePreferences() nativePreferences {
	p, _ := applicationconfig.NewService(a.configRepository).LoadPreferences()
	return p
}

func (a *App) applyNativePreferences(p config.Preferences) {
	a.aiEnabled = p.AIEnabled
	a.smoothPageJumps = p.SmoothPageJumps
	a.localeConfigured = p.LocaleConfigured
	a.localeSetting = p.Locale
	if p.LocaleConfigured {
		mygo.App.SetLocale(p.Locale)
	}
	if a.noteEditor() != nil {
		a.configureActiveNote(0)
	}
	a.accentID = appearance.Normalize(p.Accent)
	switch p.Theme {
	case config.ThemeLight:
		a.themeMode = themeLight
	case config.ThemeDark:
		a.themeMode = themeDark
	default:
		a.themeMode = themeSystem
	}
}

func (a *App) saveNativePreferences() error {
	p := config.Preferences{
		AIEnabled: a.aiEnabled, SmoothPageJumps: a.smoothPageJumps,
		Accent: appearance.Normalize(a.accentID), Locale: a.localeSetting,
		LocaleConfigured: a.localeConfigured,
	}
	switch a.themeMode {
	case themeDark:
		p.Theme = config.ThemeDark
	case themeSystem:
		p.Theme = config.ThemeSystem
	default:
		p.Theme = config.ThemeLight
	}
	return applicationconfig.NewService(a.configRepository).SavePreferences(p)
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

func (a *App) setAccent(id string) {
	id = appearance.Normalize(id)
	if appearance.Normalize(a.accentID) == id {
		return
	}
	a.accentID = id
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
	a.configureActiveNote(0)
	if err := a.saveNativePreferences(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}
