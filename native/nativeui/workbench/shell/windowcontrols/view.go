// Package windowcontrols renders native-shell controls for platforms without
// system-provided titlebar buttons.
package windowcontrols

import (
	"runtime"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/nativeui/components/icons"
)

const (
	buttonWidth  = float32(28)
	buttonHeight = float32(26)
	buttonGap    = float32(2)
	buttonRight  = float32(14)
	buttonsWidth = 3*buttonWidth + 2*buttonGap
)

// RightPadding is the space that the editor header must reserve for these
// controls, including a small visual gap.
func RightPadding() float32 { return buttonsWidth + 10 }

// Visible reports whether Muse must draw controls rather than use the macOS
// system traffic lights.
func Visible() bool { return runtime.GOOS != "darwin" }

// Model supplies the current shell state and native window operations.
type Model struct {
	Maximized bool
	Top       float32
	Minimize  func()
	Maximize  func()
	Restore   func()
	Close     func()
}

// Render paints minimize, maximize/restore, and close controls at the right
// end of the editor header.
func Render(c *ui.Context, model Model) {
	ui.Overlay(c, func() {
		row := ui.Row(c).Key("window-controls").Absolute().Right(buttonRight).
			Top(model.Top).Height(buttonHeight).Gap(buttonGap).AlignItems(ui.Center)
		row.Children(func() {
			button(c, "window-minimize", icons.WindowMinimize(), locales.WindowMinimize(), false, false, model.Minimize)
			icon, label, action := icons.WindowMaximize(), locales.WindowMaximize(), model.Maximize
			if model.Maximized {
				icon, label, action = icons.WindowRestore(), locales.WindowRestore(), model.Restore
			}
			button(c, "window-maximize", icon, label, false, true, action)
			button(c, "window-close", icons.Close(), locales.CommonClose(), true, false, model.Close)
		})
	})
}

func button(c *ui.Context, key string, icon *ui.SVG, label string, dangerous, maximize bool, run func()) {
	t := c.Theme()
	b := ui.ButtonBase(c).Key(key).Size(buttonWidth, buttonHeight).
		Radius(5).Justify(ui.Center).AlignItems(ui.Center).Padding(0).
		Cursor(ui.CursorPointer).Label(label)
	if maximize {
		b.MaximizeButton()
	} else {
		b.Tooltip(label)
	}
	color := t.Text
	switch {
	case dangerous && (b.Hovered() || b.Pressed()):
		b.Background(t.Danger)
		color = ui.RGB(255, 255, 255)
	case b.Pressed():
		b.Background(t.SurfacePressed)
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	b.Children(func() { ui.Icon(c, icon).Size(16, 16).TextColor(color) })
	if b.Clicked() && run != nil {
		run()
	}
}
