// Package searchui renders workspace search and translates interaction into
// typed actions. Query execution and navigation remain application concerns.
package workspace

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/locales"
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

// View is a current, immutable snapshot supplied by the application. Results
// are already queried and ranked; this panel never scans or copies a vault.
type View struct {
	Availability  Availability
	Error         string
	Busy          bool
	CanChangeMode bool
	Recent        []RecentItem
	Results       []Result
	SearchIcon    *ui.SVG
	FileIcon      *ui.SVG
	IconTint      ui.Color
}

type ActionKind uint8

const (
	QueryChanged ActionKind = iota + 1
	ModeChanged
	MoveSelection
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

func render(c *ui.Context, state *state, view View, dispatch func(Action)) {
	if state == nil || !state.open {
		return
	}
	count := state.viewCount(view)
	state.selected = clampSelection(state.selected, count)
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
			next := (state.selected + step + count) % count
			action := Action{Kind: MoveSelection, Index: next}
			state.apply(action)
			dispatchAction(dispatch, action)
		}
	}
	t := c.Theme()
	dialog := ui.DialogBase(c, &state.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.30))
		panel.Width(760).Height(620).MaxWidthPercent(94).MaxHeightPercent(86).Padding(0).Gap(0).Radius(14).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 18, 44, -8, ui.RGBA(0, 0, 0, 0.32)).Clip().Children(func() {
			renderHeader(c, state, view, count, dispatch, func() {
				submitted = true
				dispatchOpenSelection(state, view, dispatch)
			})
			ui.Box(c).Height(1).FillWidth().Background(t.Border)
			renderBody(c, state, view, count, dispatch)
			ui.Box(c).Height(1).FillWidth().Background(t.Border)
			renderFooter(c, state, view)
		})
	})
	if enterPressed && !submitted {
		dispatchOpenSelection(state, view, dispatch)
	}
	if dialog.Dismissed() {
		action := Action{Kind: Dismiss}
		state.apply(action)
		dispatchAction(dispatch, action)
	}
}

func renderHeader(c *ui.Context, state *state, view View, count int, dispatch func(Action), submit func()) {
	t := c.Theme()
	ui.Row(c).Height(58).Padding(0, 18).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, view.SearchIcon).Size(18, 18).TextColor(t.TextMuted)
		query := state.query
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
				next := (state.selected + step + count) % count
				action := Action{Kind: MoveSelection, Index: next}
				state.apply(action)
				dispatchAction(dispatch, action)
				return true
			})
		input.Ref(&state.inputFocus)
		if query != state.query {
			action := Action{Kind: QueryChanged, Query: query}
			state.apply(action)
			dispatchAction(dispatch, action)
		}
		if view.CanChangeMode {
			fullLabel, literalLabel := locales.WorkspaceSearchFullText(), locales.WorkspaceSearchLiteral()
			if state.mode == Literal {
				literalLabel = locales.WorkspaceSearchLiteralSelected()
			} else {
				fullLabel = locales.WorkspaceSearchFullTextSelected()
			}
			if ui.Button(c, fullLabel).Clicked() && state.mode != FullText {
				action := Action{Kind: ModeChanged, Mode: FullText}
				state.apply(action)
				dispatchAction(dispatch, action)
			}
			if ui.Button(c, literalLabel).Clicked() && state.mode != Literal {
				action := Action{Kind: ModeChanged, Mode: Literal}
				state.apply(action)
				dispatchAction(dispatch, action)
			}
		}
	})
}

func renderBody(c *ui.Context, state *state, view View, count int, dispatch func(Action)) {
	t := c.Theme()
	if strings.TrimSpace(state.query) == "" {
		renderRecent(c, state, view, dispatch)
		return
	}
	switch view.Availability {
	case VaultRequired:
		ui.Box(c).FillWidth().Grow(1).MinHeight(100).Padding(10, 12).Children(func() {
			ui.Text(c, locales.WorkspaceSearchHint()).FontSize(12).TextColor(t.TextMuted)
		})
		return
	case IndexBuilding:
		ui.Box(c).FillWidth().Grow(1).MinHeight(100).Padding(10, 12).Children(func() {
			if view.Error != "" {
				ui.Text(c, locales.WorkspaceSearchIndexUnavailable(view.Error)).Wrap()
			} else {
				ui.Text(c, locales.VaultIndexBuilding())
			}
		})
		return
	}

	if view.Error != "" {
		ui.Text(c, locales.WorkspaceSearchFailed(view.Error)).Wrap()
	}
	if view.Busy {
		ui.Text(c, locales.VaultSearching()).FontSize(11).TextColor(t.TextMuted)
	}
	if !view.Busy && view.Error == "" && len(view.Results) == 0 {
		ui.Text(c, locales.VaultSearchEmpty()).FontSize(12).TextColor(t.TextMuted)
	}
	listState := &state.results
	listState.Key = func(i int) any { return view.Results[i].Path }
	if state.revealSelection && state.selected >= 0 && state.selected < len(view.Results) {
		listState.ScrollIntoView(state.selected)
		state.revealSelection = false
	}
	showScrollbar := scrollVisible(c, state)
	ui.List(c, listState, len(view.Results), func(i int) {
		result := view.Results[i]
		renderResultRow(c, result, i == state.selected, state.query, view.FileIcon, view.IconTint, func() {
			state.selectResult(i, false)
			dispatchAction(dispatch, Action{Kind: OpenResult, Index: i, Result: result})
		})
	}).FillWidth().Grow(1).MinHeight(0).Gap(6).TrackScroll(&state.scroll).Scrollbars(showScrollbar)
}

func renderRecent(c *ui.Context, state *state, view View, dispatch func(Action)) {
	t := c.Theme()
	if len(view.Recent) == 0 {
		ui.Text(c, locales.WorkspaceRecentEmpty()).FontSize(12).TextColor(t.TextMuted)
		return
	}
	listState := &state.results
	listState.Key = func(i int) any { return view.Recent[i].Path }
	if state.revealSelection && state.selected >= 0 && state.selected < len(view.Recent) {
		listState.ScrollIntoView(state.selected)
		state.revealSelection = false
	}
	showScrollbar := scrollVisible(c, state)
	ui.List(c, listState, len(view.Recent), func(i int) {
		item := view.Recent[i]
		result := Result{Path: item.Path, Title: item.Title, PathLabel: item.PathLabel}
		renderResultRow(c, result, i == state.selected, "", view.FileIcon, view.IconTint, func() {
			state.selectResult(i, false)
			dispatchAction(dispatch, Action{Kind: OpenRecent, Index: i, Recent: item})
		})
	}).FillWidth().Grow(1).MinHeight(100).Gap(6).TrackScroll(&state.scroll).Scrollbars(showScrollbar)
}

func renderResultRow(c *ui.Context, result Result, selected bool, query string, fileIcon *ui.SVG, iconTint ui.Color, open func()) {
	t := c.Theme()
	pathLabel := result.PathLabel
	if pathLabel == "" {
		pathLabel = result.Path
	}
	row := ui.ButtonBase(c).Label(locales.WorkspaceSearchResultLabel(pathLabel)).FillWidth().Height(72).Shrink(0).Padding(8)
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

func renderFooter(c *ui.Context, state *state, view View) {
	t := c.Theme()
	ui.Row(c).Height(34).Padding(0, 16).AlignItems(ui.Center).Children(func() {
		if strings.TrimSpace(state.query) != "" && view.Availability == Ready {
			if state.mode == Literal {
				ui.Text(c, locales.SearchMatchCount(len(view.Results))).FontSize(10.5).TextColor(t.TextMuted)
			} else {
				ui.Text(c, locales.WorkspaceNoteCount(len(view.Results))).FontSize(10.5).TextColor(t.TextMuted)
			}
		} else if strings.TrimSpace(state.query) == "" {
			ui.Text(c, locales.WorkspaceRecentNotes()).FontSize(10.5).TextColor(t.TextMuted)
		}
		ui.Spacer(c)
		ui.Text(c, locales.WorkspaceQuickKeys()).FontSize(10.5).TextColor(t.TextMuted)
	})
}

func scrollVisible(c *ui.Context, state *state) bool {
	visible := state.indicator.Visible(c.Now(), state.scroll.X, state.scroll.Y)
	if visible {
		c.After(state.indicator.Remaining(c.Now()))
	}
	return visible
}

func clampSelection(selected, count int) int {
	if count == 0 {
		return 0
	}
	return max(0, min(selected, count-1))
}

func dispatchOpenSelection(state *state, view View, dispatch func(Action)) {
	selected := state.selected
	if strings.TrimSpace(state.query) == "" {
		if selected >= 0 && selected < len(view.Recent) {
			dispatchAction(dispatch, Action{Kind: OpenRecent, Index: selected, Recent: view.Recent[selected]})
		}
		return
	}
	if view.Availability == Ready && selected >= 0 && selected < len(view.Results) {
		dispatchAction(dispatch, Action{Kind: OpenResult, Index: selected, Result: view.Results[selected]})
	}
}

func dispatchAction(dispatch func(Action), action Action) {
	if dispatch != nil {
		dispatch(action)
	}
}
