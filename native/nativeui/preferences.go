package nativeui

import (
	applicationconfig "muse/application/config"
	"muse/config"
	"muse/locales"
	"muse/theme"

	"github.com/egoist/mygo"
)

const (
	themeLight = iota
	themeDark
	themeSystem
)

type nativePreferences = config.Preferences

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
	a.accentID = theme.Normalize(p.Accent)
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
		Accent: theme.Normalize(a.accentID), Locale: a.localeSetting,
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
