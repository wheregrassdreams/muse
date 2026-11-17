package nativeui

import (
	"github.com/egoist/mygo/ui"

	"muse/nativeui/components/windowcontrols"
)

// headerVeil adapts the shell and active-document state to the reusable
// window-controls renderer. The component owns the contents of its cache;
// App retains only the opaque cache for the lifetime of the window.
func (a *App) headerVeil(c *ui.Context) {
	var scrollY float32
	if editor := a.noteEditor(); editor != nil {
		scrollY = editor.NavigationCommands().ScrollY()
	}
	windowcontrols.RenderHeaderFade(c, &a.headerFadeCache, a.headerFade, a.headerH(c), scrollY, a.windowBG())
}
