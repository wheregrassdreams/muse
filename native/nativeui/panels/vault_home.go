package nativeui

import (
	"github.com/egoist/mygo/ui"
	"muse/locales"
)

// vaultHome is the blank landing page shown after opening a workspace before
// the user chooses a note. It keeps the available actions visible, like the
// empty state in Obsidian, without offering a whiteboard action.
func (a *App) vaultHome(c *ui.Context) {
	ui.Column(c).Fill().Justify(ui.Center).AlignItems(ui.Center).Gap(12).Children(func() {
		vaultHomeAction(c, locales.DocumentNew(), iconEdit, "⌘ N", func() {
			a.newFileIn(a.createTarget())
		})
		vaultHomeAction(c, locales.FilesOpenFileMenu(), iconSearch, "", a.openFileDialog)
	})
}

func vaultHomeAction(c *ui.Context, label string, icon *ui.SVG, shortcut string, run func()) {
	t := c.Theme()
	b := ui.ButtonBase(c).Width(240).Height(56).Shrink(0).Padding(10).Radius(12).
		Label(label)
	switch {
	case b.Pressed():
		b.Background(t.SurfacePressed)
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	b.OnClick(run)
	b.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Icon(c, icon).Size(18, 18).TextColor(t.TextMuted)
			ui.Text(c, label).FontSize(16).FontWeight(600).TextColor(t.Text)
			ui.Spacer(c)
			if shortcut != "" {
				ui.Text(c, shortcut).FontSize(12).TextColor(t.TextMuted)
			}
		})
	})
}
