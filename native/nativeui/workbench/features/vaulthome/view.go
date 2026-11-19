// Package vaulthome renders the vault landing page. It owns the view
// model and action vocabulary; the application shell handles those actions.
package vaulthome

import (
	"github.com/egoist/mygo/ui"
	"muse/locales"
)

type Action uint8

const (
	NewDocument Action = iota + 1
	OpenFile
)

type ViewModel struct {
	NewLabel, OpenLabel string
	NewShortcut         string
	NewIcon, OpenIcon   *ui.SVG
}

func DefaultViewModel(newIcon, openIcon *ui.SVG) ViewModel {
	return ViewModel{
		NewLabel: locales.DocumentNew(), OpenLabel: locales.FilesOpenFileMenu(),
		NewShortcut: "⌘ N", NewIcon: newIcon, OpenIcon: openIcon,
	}
}

func Render(c *ui.Context, model ViewModel, dispatch func(Action)) {
	ui.Column(c).Fill().Justify(ui.Center).AlignItems(ui.Center).Gap(12).Children(func() {
		action(c, model.NewLabel, model.NewIcon, model.NewShortcut, NewDocument, dispatch)
		action(c, model.OpenLabel, model.OpenIcon, "", OpenFile, dispatch)
	})
}

func action(c *ui.Context, label string, icon *ui.SVG, shortcut string, kind Action, dispatch func(Action)) {
	t := c.Theme()
	b := ui.ButtonBase(c).Width(240).Height(56).Shrink(0).Padding(10).Radius(12).Label(label)
	switch {
	case b.Pressed():
		b.Background(t.SurfacePressed)
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	b.OnClick(func() {
		if dispatch != nil {
			dispatch(kind)
		}
	})
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
