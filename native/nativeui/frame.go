package nativeui

// The window's frame on Windows and Linux: the system's buttons are taken
// away, and the view draws Windows 11's minimize, maximize or restore, and
// close at the top-right, where those platforms keep them — three flat
// glyphs on the editor card's own surface, the close one turning red under
// the pointer. macOS keeps its traffic lights, so there is nothing to draw
// there.

import (
	"runtime"

	"muse/locales"

	"github.com/egoist/mygo/ui"
)

const (
	// The three flat buttons at the right end of the editor header's line,
	// 28 by 26 with 2 between them, where Windows 11 keeps its own.
	windowButtonW     = float32(28)
	windowButtonH     = float32(26)
	windowButtonGap   = float32(2)
	windowButtonRight = float32(14)

	// windowButtonsW is the room the three take, and windowButtonsRoom the
	// room the editor header keeps clear for them at its right.
	windowButtonsW    = 3*windowButtonW + 2*windowButtonGap
	windowButtonsRoom = windowButtonsW + 10
)

// controlsAtRight reports whether the view draws the window's controls at the
// top-right corner, as Windows and Linux keep them, rather than leaving them
// to the system at macOS's top-left.
var controlsAtRight = runtime.GOOS != "darwin"

// windowControls draws the window's own controls, and only on Windows and
// Linux: macOS's traffic lights are the system's, so there is nothing to
// draw there.
func (a *App) windowControls(c *ui.Context) {
	if !controlsAtRight || a.win == nil {
		return
	}
	ui.Overlay(c, func() {
		a.windowButtons(c)
	})
}

// windowButtons draws minimize, maximize or restore and close at the right
// end of the editor header's line, over the card it sits on.
func (a *App) windowButtons(c *ui.Context) {
	win := a.win
	row := ui.Row(c).Key("window-controls").Absolute().
		Right(windowButtonRight).
		Top(canvasPadding + cardBorder + (editorHeaderH-windowButtonH)/2).
		Height(windowButtonH).Gap(windowButtonGap).AlignItems(ui.Center)
	row.Children(func() {
		windowButton(c, "window-minimize", iconWindowMinimize, locales.WindowMinimize(), false, false, win.Minimize)

		maximized := win.IsMaximized()
		icon, label := iconWindowMaximize, locales.WindowMaximize()
		if maximized {
			icon, label = iconWindowRestore, locales.WindowRestore()
		}
		windowButton(c, "window-maximize", icon, label, false, true, func() {
			if maximized {
				win.Unmaximize()
			} else {
				win.Maximize()
			}
		})

		windowButton(c, "window-close", iconClose, locales.CommonClose(), true, false, win.Close)
	})
}

// windowButton is one of the window's buttons on Windows and Linux: a quiet
// glyph that gets a wash under the pointer, and the close one a red face, as
// Windows 11's does.
func windowButton(c *ui.Context, key string, icon *ui.SVG, label string, dangerous, maximize bool, run func()) {
	t := c.Theme()
	b := ui.ButtonBase(c).Key(key).Size(windowButtonW, windowButtonH).
		Radius(5).Justify(ui.Center).AlignItems(ui.Center).Padding(0).
		Cursor(ui.CursorPointer).Label(label)
	if maximize {
		// The system's maximize button for the pointer: on Windows 11 it
		// brings the snap layouts while the pointer rests there, and no
		// tooltip covers them.
		b.MaximizeButton()
	} else {
		b.Tooltip(label)
	}
	col := iconTint(t)
	switch {
	case dangerous && (b.Hovered() || b.Pressed()):
		b.Background(t.Danger)
		col = ui.RGB(255, 255, 255)
	case b.Pressed():
		b.Background(t.SurfacePressed)
		col = t.Text
	case b.Hovered():
		b.Background(t.SurfaceHover)
		col = t.Text
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(col)
	})
	if b.Clicked() {
		run()
	}
}
