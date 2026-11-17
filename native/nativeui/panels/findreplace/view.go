package findreplace

import (
	"github.com/egoist/mygo/ui"
	"muse/editor/findreplace"
	"muse/locales"
)

func Render(c *ui.Context, handle *Handle, dispatch func(Action)) {
	state := handle.ensureState()
	if state == nil {
		return
	}
	t := c.Theme()
	ui.Column(c).Fill().Padding(10).Gap(8).Children(func() {
		query := state.query
		ui.TextInput(c, &query).Placeholder(locales.DocumentFindPlaceholder()).Label(locales.DocumentFind())
		if query != state.query {
			action := Action{Kind: QueryChanged, Query: query}
			state.apply(action)
			dispatch(action)
		}
		if state.query == "" {
			ui.Text(c, locales.DocumentFindHint()).FontSize(11.5).TextColor(t.TextMuted).Wrap()
			return
		}
		if len(state.matches) == 0 {
			ui.Text(c, locales.SearchNoMatches()).FontSize(11.5).TextColor(t.TextMuted)
			return
		}
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			replacement := state.replacement
			ui.TextInput(c, &replacement).Placeholder(locales.DocumentReplace()).Label(locales.DocumentReplace()).Grow(1).MinWidth(0)
			if replacement != state.replacement {
				action := Action{Kind: ReplacementChanged, Text: replacement}
				state.apply(action)
				dispatch(action)
			}
			if ui.Button(c, locales.DocumentReplaceOne()).Clicked() {
				dispatch(Action{Kind: ReplaceOne})
			}
			if ui.Button(c, locales.CommonAll()).Tooltip(locales.DocumentReplaceAll()).Clicked() {
				dispatch(Action{Kind: ReplaceAll})
			}
		})
		ui.Text(c, locales.SearchMatchCount(len(state.matches))).FontSize(11).TextColor(t.TextMuted)
		showScrollbar := state.indicator.Visible(c.Now(), state.scroll.X, state.scroll.Y)
		if showScrollbar {
			c.After(state.indicator.Remaining(c.Now()))
		}
		ui.List(c, &state.list, len(state.matches), func(i int) {
			row := ui.Row(c).Height(28).Radius(6).Padding(0, 8).Cursor(ui.CursorPointer)
			switch {
			case i == state.selected:
				if t.Dark {
					row.Background(ui.RGBA(255, 255, 255, 0.08))
				} else {
					row.Background(t.Background)
					row.Shadow(0, 0.5, 2, 0, ui.RGBA(0, 0, 0, 0.06))
				}
			case row.Hovered():
				row.Background(t.SurfaceHover)
			}
			row.Children(func() {
				before, hit, after := findreplace.SnippetParts(state.matches[i].Snippet, state.query)
				ui.Text(c, before).FontSize(11.5).TextColor(t.Text)
				if hit != "" {
					ui.Text(c, hit).FontSize(11.5).FontWeight(620).TextColor(t.Accent)
				}
				ui.Text(c, after).FontSize(11.5).TextColor(t.Text)
			})
			row.OnClick(func() { dispatch(Action{Kind: OpenMatch, Index: i}) })
		}).Grow(1).Gap(1).TrackScroll(&state.scroll).Scrollbars(showScrollbar)
	})
}
