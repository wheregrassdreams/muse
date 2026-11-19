package backlinks

import (
	"github.com/egoist/mygo/ui"
	"muse/locales"
	scrollui "muse/nativeui/ui/scroll"
	"muse/ui/scrollbar"
)

type ActionKind uint8

const (
	Dismiss ActionKind = iota + 1
	OpenSource
	OpenTarget
)

type Action struct {
	Kind   ActionKind
	Path   string
	Offset int
	Target string
}

func Render(c *ui.Context, model Model, scroll *ui.ScrollState, indicator *scrollbar.Indicator, dispatch func(Action)) {
	t := c.Theme()
	backdrop := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Center().Modal().Background(ui.RGBA(0, 0, 0, 0.32))
	backdrop.Children(func() {
		ui.Column(c).Width(540).MaxHeight(600).
			Padding(20).Gap(10).Background(t.Background).Radius(12).
			Border(1, t.Border).Shadow(0, 12, 32, -6, ui.RGBA(0, 0, 0, 0.28)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.VaultBacklinksTitle()).FontWeight(700).Grow(1)
				ui.Button(c, locales.CommonClose()).OnClick(func() { dispatch(Action{Kind: Dismiss}) })
			})
			if !model.Ready {
				ui.Text(c, locales.VaultIndexBuildingShort())
				return
			}
			scrollui.QuietScroll(c, scroll, indicator).Height(360).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					ui.Text(c, locales.VaultLinksToHere())
					if model.Busy {
						ui.Text(c, locales.VaultFindingBacklinks())
					}
					if len(model.Incoming) == 0 {
						ui.Text(c, locales.VaultNoBacklinks())
					}
					for _, link := range model.Incoming {
						link := link
						label := link.Source
						if link.Count > 1 {
							label += locales.VaultBacklinkCountSuffix(link.Count)
						}
						if ui.Button(c, label).Label(locales.VaultBacklinkSourceLabel(label)).Clicked() {
							dispatch(Action{Kind: OpenSource, Path: link.Source, Offset: link.Start})
						}
					}
					ui.Text(c, locales.VaultOutgoingLinks())
					for _, link := range model.Outgoing {
						link := link
						if ui.Button(c, link.Target).Label(locales.VaultOpenLinkLabel(link.Target)).Clicked() {
							dispatch(Action{Kind: OpenTarget, Target: link.Target})
						}
					}
				})
			})
		})
	})
	if backdrop.OverlayShortcut(0, ui.KeyEscape) || backdrop.PressedOutside() {
		dispatch(Action{Kind: Dismiss})
	}
}
