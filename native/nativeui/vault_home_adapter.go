package nativeui

import (
	"github.com/egoist/mygo/ui"
	componenticons "muse/nativeui/components/icons"
	"muse/nativeui/panels/vaulthome"
)

func (a *App) vaultHome(c *ui.Context) {
	model := vaulthome.DefaultViewModel(componenticons.Edit(), componenticons.Search())
	vaulthome.Render(c, model, func(action vaulthome.Action) {
		switch action {
		case vaulthome.NewDocument:
			a.newFileIn(a.fileTreeState.CreateTarget(a.root, a.selectedPath, a.catalogIsDirectory(a.selectedPath)))
		case vaulthome.OpenFile:
			a.openFileDialog()
		}
	})
}
