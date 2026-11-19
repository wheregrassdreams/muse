// Package naviconpicker renders the custom navigation icon picker. Search,
// persistence and application state remain with its caller through typed
// actions and a narrow view model.
package naviconpicker

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

const (
	columns      = 5
	rowHeight    = float32(58)
	visibleRows  = 6
	pickerHeight = float32(444)
)

type Option struct {
	ID, Label string
	Icon      *ui.SVG
}

type ViewModel struct {
	Path    string
	Query   string
	Scroll  *ui.ScrollState
	Results []Option
	Loading bool
}

type ActionKind uint8

const (
	QueryChanged ActionKind = iota + 1
	ScrollChanged
	Choose
	Close
	RestoreDefault
)

type Action struct {
	Kind   ActionKind
	Query  string
	Scroll ui.ScrollState
	ID     string
}

func Render(c *ui.Context, model ViewModel, dispatch func(Action)) {
	t := c.Theme()
	ui.Column(c).Key("nav-icon-picker").Width(336).Height(pickerHeight).Gap(8).Padding(14).Radius(14).
		Background(t.Background).Border(1, t.Border).
		Shadow(0, 14, 34, -4, ui.RGBA(0, 0, 0, 0.22)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.IconPickerTitle()).FontSize(14).FontWeight(650)
			ui.Spacer(c)
			ui.Text(c, filepath.Base(model.Path)).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			if ui.Button(c, locales.CommonClose()).Clicked() {
				dispatchAction(dispatch, Action{Kind: Close})
			}
		})
		before := model.Query
		query := before
		ui.TextInput(c, &query).Placeholder(locales.IconPickerSearchPlaceholder()).Label(locales.IconPickerSearch()).AutoFocus()
		if query != before {
			dispatchAction(dispatch, Action{Kind: QueryChanged, Query: query})
		}
		countLabel := locales.IconPickerAllIcons()
		if query != "" {
			countLabel = locales.IconPickerResults()
		}
		ui.Text(c, locales.IconPickerResultCount(countLabel, len(model.Results))).FontSize(10).TextColor(t.TextMuted)
		rows := (len(model.Results) + columns - 1) / columns
		scroll := model.Scroll
		if scroll == nil {
			scroll = &ui.ScrollState{}
		}
		firstRow := max(int(scroll.Y/rowHeight)-1, 0)
		lastRow := min(firstRow+visibleRows+2, rows)
		ui.Scroll(c).FillWidth().Height(312).TrackScroll(scroll).Scrollbars(scroll.MaxY > 0).Children(func() {
			ui.Column(c).FillWidth().Height(float32(rows) * rowHeight).Shrink(0).Children(func() {
				if firstRow > 0 {
					ui.Box(c).FillWidth().Height(float32(firstRow) * rowHeight).Shrink(0)
				}
				for row := firstRow; row < lastRow; row++ {
					start := row * columns
					end := min(start+columns, len(model.Results))
					ui.Row(c).Height(rowHeight).Shrink(0).AlignItems(ui.Center).Gap(4).Children(func() {
						for _, option := range model.Results[start:end] {
							option := option
							button := ui.ButtonBase(c).Size(56, 54).Radius(8).Padding(3).
								Justify(ui.Center).AlignItems(ui.Center).Label(option.Label).Tooltip(option.Label)
							if button.Hovered() {
								button.Background(t.SurfaceHover)
							}
							button.OnClick(func() { dispatchAction(dispatch, Action{Kind: Choose, ID: option.ID}) }).Children(func() {
								ui.Column(c).Gap(2).AlignItems(ui.Center).Children(func() {
									ui.Icon(c, option.Icon).Size(19, 19).TextColor(t.Text)
									ui.Text(c, option.Label).FontSize(8).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
								})
							})
						}
					})
				}
				if lastRow < rows {
					ui.Box(c).FillWidth().Height(float32(rows-lastRow) * rowHeight).Shrink(0)
				}
			})
		})
		if model.Loading {
			ui.Text(c, locales.IconPickerLoading()).FontSize(10).TextColor(t.TextMuted)
		} else if len(model.Results) == 0 {
			ui.Text(c, locales.IconPickerNoMatch()).FontSize(10).TextColor(t.TextMuted)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, locales.CommonRestoreDefaultIcon()).Clicked() {
				dispatchAction(dispatch, Action{Kind: RestoreDefault})
			}
			if ui.Button(c, locales.CommonCancel()).Clicked() {
				dispatchAction(dispatch, Action{Kind: Close})
			}
		})
	})
}

// renderPopover keeps the toolkit's open pointer private to the feature host.
func renderPopover(c *ui.Context, anchor ui.Element, state *state, dispatch func(Action)) {
	if state == nil {
		return
	}
	state.mu.Lock()
	open := state.open
	model := ViewModel{
		Path: state.path, Query: state.query, Scroll: &state.scroll,
		Results: append([]Option(nil), state.results...), Loading: state.loading,
	}
	state.mu.Unlock()
	ui.Popover(c, anchor, &open, func() {
		Render(c, model, func(action Action) {
			if action.Kind == Close || action.Kind == Choose || action.Kind == RestoreDefault {
				open = false
				state.Close()
			}
			if dispatch != nil {
				dispatch(action)
			}
		})
	})
	state.mu.Lock()
	if state.open && !open {
		state.request++
		state.loading = false
	}
	state.open = open
	state.mu.Unlock()
}

func dispatchAction(dispatch func(Action), action Action) {
	if dispatch != nil {
		dispatch(action)
	}
}
