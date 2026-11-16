package nativeui

import (
	"github.com/egoist/mygo/ui"
	"muse/nativeui/panels/vaulthome"
)

func (a *App) vaultHome(c *ui.Context) {
	model := vaulthome.DefaultViewModel(iconEdit, iconSearch)
	vaulthome.Render(c, model, func(action vaulthome.Action) {
		switch action {
		case vaulthome.NewDocument:
			a.newFileIn(a.createTarget())
		case vaulthome.OpenFile:
			a.openFileDialog()
		}
	})
}
