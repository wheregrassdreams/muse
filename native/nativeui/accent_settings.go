package nativeui

import (
	"muse/locales"
	appearance "muse/theme"
)

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

func accentLocalizedName(id string) string {
	switch id {
	case "orange":
		return locales.AccentOrange()
	case "yellow":
		return locales.AccentYellow()
	case "green":
		return locales.AccentGreen()
	case "blue":
		return locales.AccentBlue()
	case "purple":
		return locales.AccentPurple()
	case "pink":
		return locales.AccentPink()
	default:
		return locales.AccentRed()
	}
}
