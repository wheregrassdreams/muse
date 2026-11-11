package nativeui

import (
	"github.com/egoist/mygo/ui"
	"muse/appearance"
)

func (a *App) setAccent(id string) {
	id = appearance.Normalize(id)
	if appearance.Normalize(a.accentID) == id {
		return
	}
	a.accentID = id
	if err := a.saveNativePreferences(); err != nil {
		a.notice("保存设置失败：" + err.Error())
	}
	a.invalidate()
}

func (a *App) accentSettings(c *ui.Context) {
	name := "红色"
	for _, p := range appearance.Presets {
		if p.ID == appearance.Normalize(a.accentID) {
			name = p.Name
			break
		}
	}
	settingsRow(c, "重点色", "用于链接、按钮、光标与选中区域。当前："+name, func() {
		for _, preset := range appearance.Presets {
			preset := preset
			selected := appearance.Normalize(a.accentID) == preset.ID
			palette := appearance.From(preset.Color, c.Theme().Dark)
			button := ui.ButtonBase(c).Size(30, 30).Radius(15).Padding(3).Label("重点色：" + preset.Name)
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
