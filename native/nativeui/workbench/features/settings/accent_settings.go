package settings

import (
	"muse/locales"
)

// LocalizedAccentName projects the display label for an accent preset.
func LocalizedAccentName(id string) string {
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
