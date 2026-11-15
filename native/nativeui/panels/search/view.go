// Package search renders workspace search UI and translates input into typed
// actions. Query execution, recent-note discovery, candidate/ranking policy,
// and navigation remain with application/search callers.
package search

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/ui/scrollbar"
)

type Mode uint8

const (
	FullText Mode = iota
	Literal
)

type Availability uint8

const (
	Ready Availability = iota
	VaultRequired
	IndexBuilding
)

type RecentItem struct {
	Path, Title, PathLabel string
}

type Result struct {
	Path, Title, PathLabel, Snippet string
	Offset                          int
	Revision                        string
}

type ScrollPort struct {
	State     *ui.ScrollState
	Results   *ui.ListState
	Indicator *scrollbar.Indicator
}

type ViewModel struct {
	Open            *bool
	Query           string
	Mode            Mode
	Availability    Availability
	Error           string
	Busy            bool
	CanChangeMode   bool
	Recent          []RecentItem
	Results         []Result
	Selected        int
	RevealSelection bool
	InputFocus      *ui.Handle
	SearchIcon      *ui.SVG
	FileIcon        *ui.SVG
	IconTint        ui.Color
	Scroll          ScrollPort
}

type ActionKind uint8

const (
	QueryChanged ActionKind = iota + 1
	ModeChanged
	MoveSelection
	SelectionRevealed
	OpenRecent
	OpenResult
	Dismiss
)

type Action struct {
	Kind   ActionKind
	Query  string
	Mode   Mode
	Index  int
	Recent RecentItem
	Result Result
}

func Render(c *ui.Context, model ViewModel, dispatch func(Action)) {
	if model.Open == nil || !*model.Open {
		return
	}
	count := rowCount(model)
	selected := clampSelection(model.Selected, count)
	enterPressed := c.Shortcut(0, ui.KeyEnter)
	submitted := false
	if count > 0 {
		step := 0
		if c.Shortcut(0, ui.KeyUp) {
			step = -1
		}
		if c.Shortcut(0, ui.KeyDown) {
			step = 1
		}
		if step != 0 {
			selected = (selected + step + count) % count
			dispatchAction(dispatch, Action{Kind: MoveSelection, Index: selected})
		}
	}
	t := c.Theme()
	dialog := ui.DialogBase(c, model.Open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.30))
		panel.Width(760).Height(620).MaxWidthPercent(94).MaxHeightPercent(86).Padding(0).Gap(0).Radius(14).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 18, 44, -8, ui.RGBA(0, 0, 0, 0.32)).Clip().Children(func() {
			renderHeader(c, model, selected, count, dispatch, func() {
				submitted = true
				dispatchOpenSelection(model, selected, dispatch)
			})
			ui.Box(c).Height(1).FillWidth().Background(t.Border)
			renderBody(c, model, selected, count, dispatch)
			ui.Box(c).Height(1).FillWidth().Background(t.Border)
			renderFooter(c, model)
		})
	})
	if enterPressed && !submitted {
		dispatchOpenSelection(model, selected, dispatch)
	}
	if dialog.Dismissed() {
		dispatchAction(dispatch, Action{Kind: Dismiss})
	}
}

func renderHeader(c *ui.Context, model ViewModel, selected, count int, dispatch func(Action), submit func()) {
	t := c.Theme()
	ui.Row(c).Height(58).Padding(0, 18).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, model.SearchIcon).Size(18, 18).TextColor(t.TextMuted)
		query := model.Query
		input := ui.TextInputBase(c, &query).Grow(1).MinWidth(0).
			Placeholder(locales.WorkspaceSearchPlaceholder()).Label(locales.MenuSearchWorkspace()).AutoFocus().
			OnSubmit(submit).
			OnTextKey(func(k ui.TextKey) bool {
				if k.Mods != ui.Ctrl || (k.Key != ui.KeyN && k.Key != ui.KeyP) || count == 0 {
					return false
				}
				step := 1
				if k.Key == ui.KeyP {
					step = -1
				}
				next := (selected + step + count) % count
				dispatchAction(dispatch, Action{Kind: MoveSelection, Index: next})
				return true
			})
		if model.InputFocus != nil {
			input.Ref(model.InputFocus)
		}
		if query != model.Query {
			dispatchAction(dispatch, Action{Kind: QueryChanged, Query: query})
		}
		if model.CanChangeMode {
			fullLabel, literalLabel := locales.WorkspaceSearchFullText(), locales.WorkspaceSearchLiteral()
			if model.Mode == Literal {
				literalLabel = locales.WorkspaceSearchLiteralSelected()
			} else {
				fullLabel = locales.WorkspaceSearchFullTextSelected()
			}
			if ui.Button(c, fullLabel).Clicked() && model.Mode != FullText {
				dispatchAction(dispatch, Action{Kind: ModeChanged, Mode: FullText})
			}
			if ui.Button(c, literalLabel).Clicked() && model.Mode != Literal {
				dispatchAction(dispatch, Action{Kind: ModeChanged, Mode: Literal})
			}
		}
	})
}

func renderBody(c *ui.Context, model ViewModel, selected, count int, dispatch func(Action)) {
	t := c.Theme()
	if strings.TrimSpace(model.Query) == "" {
		renderRecent(c, model, selected, dispatch)
		return
	}
	switch model.Availability {
	case VaultRequired:
		ui.Box(c).FillWidth().Grow(1).MinHeight(100).Padding(10, 12).Children(func() {
			ui.Text(c, locales.WorkspaceSearchHint()).FontSize(12).TextColor(t.TextMuted)
		})
		return
	case IndexBuilding:
		ui.Box(c).FillWidth().Grow(1).MinHeight(100).Padding(10, 12).Children(func() {
			if model.Error != "" {
				ui.Text(c, locales.WorkspaceSearchIndexUnavailable(model.Error)).Wrap()
			} else {
				ui.Text(c, locales.VaultIndexBuilding())
			}
		})
		return
	}

	if model.Error != "" {
		ui.Text(c, locales.WorkspaceSearchFailed(model.Error)).Wrap()
	}
	if model.Busy {
		ui.Text(c, locales.VaultSearching()).FontSize(11).TextColor(t.TextMuted)
	}
	if !model.Busy && model.Error == "" && len(model.Results) == 0 {
		ui.Text(c, locales.VaultSearchEmpty()).FontSize(12).TextColor(t.TextMuted)
	}
	listState := model.Scroll.Results
	if listState == nil {
		listState = &ui.ListState{}
	}
	if model.RevealSelection && selected >= 0 && selected < len(model.Results) {
		listState.ScrollIntoView(selected)
		dispatchAction(dispatch, Action{Kind: SelectionRevealed})
	}
	state := model.Scroll.State
	if state == nil {
		state = &ui.ScrollState{}
	}
	showScrollbar := scrollVisible(c, model.Scroll)
	ui.List(c, listState, len(model.Results), func(i int) {
		result := model.Results[i]
		renderResultRow(c, result, i == selected, model.Query, model.FileIcon, model.IconTint, func() {
			dispatchAction(dispatch, Action{Kind: OpenResult, Index: i, Result: result})
		})
	}).FillWidth().Grow(1).MinHeight(0).Gap(6).TrackScroll(state).Scrollbars(showScrollbar)
}

func renderRecent(c *ui.Context, model ViewModel, selected int, dispatch func(Action)) {
	t := c.Theme()
	state := model.Scroll.State
	if state == nil {
		state = &ui.ScrollState{}
	}
	showScrollbar := scrollVisible(c, model.Scroll)
	ui.Scroll(c).FillWidth().Grow(1).MinHeight(100).TrackScroll(state).Scrollbars(showScrollbar).
		Padding(10, 12).Children(func() {
		if len(model.Recent) == 0 {
			ui.Text(c, locales.WorkspaceRecentEmpty()).FontSize(12).TextColor(t.TextMuted)
		}
		for i, item := range model.Recent {
			item := item
			renderResultRow(c, Result{Path: item.Path, Title: item.Title, PathLabel: item.PathLabel}, i == selected, "", model.FileIcon, model.IconTint, func() {
				dispatchAction(dispatch, Action{Kind: OpenRecent, Index: i, Recent: item})
			})
		}
	})
}

func renderResultRow(c *ui.Context, result Result, selected bool, query string, fileIcon *ui.SVG, iconTint ui.Color, open func()) {
	t := c.Theme()
	row := ui.ButtonBase(c).Label(locales.WorkspaceSearchResultLabel(result.Path)).FillWidth().Height(72).Shrink(0).Padding(8)
	if selected || row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.OnClick(open).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Icon(c, fileIcon).Size(16, 16).TextColor(iconTint)
			ui.Column(c).FillWidth().AlignItems(ui.Start).Gap(3).Children(func() {
				searchText(c, result.Title, query, t.Text, 13).FontWeight(600)
				searchText(c, result.PathLabel, query, t.TextMuted, 10.5)
				searchText(c, strings.Join(strings.Fields(result.Snippet), " "), query, t.TextMuted, 11.5)
			})
		})
	})
}

func renderFooter(c *ui.Context, model ViewModel) {
	t := c.Theme()
	ui.Row(c).Height(34).Padding(0, 16).AlignItems(ui.Center).Children(func() {
		if strings.TrimSpace(model.Query) != "" && model.Availability == Ready {
			if model.Mode == Literal {
				ui.Text(c, locales.SearchMatchCount(len(model.Results))).FontSize(10.5).TextColor(t.TextMuted)
			} else {
				ui.Text(c, locales.WorkspaceNoteCount(len(model.Results))).FontSize(10.5).TextColor(t.TextMuted)
			}
		} else if strings.TrimSpace(model.Query) == "" {
			ui.Text(c, locales.WorkspaceRecentNotes()).FontSize(10.5).TextColor(t.TextMuted)
		}
		ui.Spacer(c)
		ui.Text(c, locales.WorkspaceQuickKeys()).FontSize(10.5).TextColor(t.TextMuted)
	})
}

func scrollVisible(c *ui.Context, port ScrollPort) bool {
	if port.State == nil || port.Indicator == nil {
		return false
	}
	visible := port.Indicator.Visible(c.Now(), port.State.X, port.State.Y)
	if visible {
		c.After(port.Indicator.Remaining(c.Now()))
	}
	return visible
}

func rowCount(model ViewModel) int {
	if strings.TrimSpace(model.Query) == "" {
		return len(model.Recent)
	}
	if model.Availability != Ready {
		return 0
	}
	return len(model.Results)
}

func clampSelection(selected, count int) int {
	if count == 0 {
		return 0
	}
	return max(0, min(selected, count-1))
}

func dispatchOpenSelection(model ViewModel, selected int, dispatch func(Action)) {
	if strings.TrimSpace(model.Query) == "" {
		if selected >= 0 && selected < len(model.Recent) {
			dispatchAction(dispatch, Action{Kind: OpenRecent, Index: selected, Recent: model.Recent[selected]})
		}
		return
	}
	if model.Availability == Ready && selected >= 0 && selected < len(model.Results) {
		dispatchAction(dispatch, Action{Kind: OpenResult, Index: selected, Result: model.Results[selected]})
	}
}

func dispatchAction(dispatch func(Action), action Action) {
	if dispatch != nil {
		dispatch(action)
	}
}
