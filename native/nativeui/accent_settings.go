package nativeui

import (
	"github.com/egoist/mygo/ui"
	"muse/appearance"
	"muse/locales"
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

func (a *App) accentSettings(c *ui.Context) {
	name := accentLocalizedName(appearance.Normalize(a.accentID))
	for _, p := range appearance.Presets {
		if p.ID == appearance.Normalize(a.accentID) {
			name = accentLocalizedName(p.ID)
			break
		}
	}
	settingsRow(c, locales.AccentColor(), locales.AccentColorDetail(name), func() {
		for _, preset := range appearance.Presets {
			preset := preset
			selected := appearance.Normalize(a.accentID) == preset.ID
			palette := appearance.From(preset.Color, c.Theme().Dark)
			button := ui.ButtonBase(c).Size(30, 30).Radius(15).Padding(3).Label(locales.AccentNamed(accentLocalizedName(preset.ID)))
			if selected {
				button.Border(2, c.Theme().Accent)
			} else if button.Hovered() {
				button.Background(c.Theme().SurfaceHover)
			}
			button.OnClick(func() { a.setAccent(preset.ID) }).Children(func() {
				ui.Row(c).Size(24, 24).Radius(12).Background(palette.Accent).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					if selected {
						ui.Text(c, "✓").FontSize(14).FontWeight(600).TextColor(palette.OnAccent)
					}
				})
			})
		}
	})
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
