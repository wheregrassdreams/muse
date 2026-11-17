package nativeui

import (
	"muse/nativeui/components/windowcontrols"

	"github.com/egoist/mygo/ui"
)

// windowControls adapts the application window to the reusable shell control
// component. Window operations stay in the composition root.
func (a *App) windowControls(c *ui.Context) {
	win := a.win
	if win == nil || !windowcontrols.Visible() {
		return
	}
	maximized := win.IsMaximized()
	windowcontrols.Render(c, windowcontrols.Model{
		Top:       canvasPadding + cardBorder + (editorHeaderH-26)/2,
		Maximized: maximized,
		Minimize:  win.Minimize,
		Maximize:  win.Maximize,
		Restore:   win.Unmaximize,
		Close:     win.Close,
	})
}
