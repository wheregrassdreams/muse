// Package quickopen renders the Quick Open palette. Search ranking and file
// catalog construction are provided by its caller; this package owns only
// presentation and input translation.
package quickopen

import (
	"github.com/egoist/mygo/ui"
	"muse/locales"
)

type Row struct {
	Path, Title               string
	PathText                  string
	TitleMatches, PathMatches []int
}

type Rows interface {
	Len() int
	At(int) Row
}

// Revision describes changes to the candidate catalog independent of the
// current query. The controller refreshes its row projection when either the
// query or this revision changes.
type Revision struct {
	Catalog uint64
	Index   uint64
}

// CandidateSource supplies pre-ranked rows from the application/search
// layers. Quick Open owns query presentation and selection, not matching.
type CandidateSource interface {
	Rows(query string) Rows
	Revision() Revision
}

type ViewModel struct {
	State    state
	Rows     Rows
	FileIcon *ui.SVG
	IconTint ui.Color
}

type state struct {
	Open     bool
	Query    string
	Selected int
}

type ActionKind uint8

const (
	QueryChanged ActionKind = iota + 1
	MoveSelection
	OpenSelection
	Dismiss
)

type Action struct {
	Kind  ActionKind
	Query string
	Index int
}

func Render(c *ui.Context, model ViewModel, dispatch func(Action)) {
	if c.Shortcut(0, ui.KeyEscape) {
		dispatch(Action{Kind: Dismiss})
		return
	}
	t := c.Theme()
	ui.Box(c).Fill().Background(ui.RGBA(0, 0, 0, 0.32)).Children(func() {
		ui.Column(c).Width(560).Margin(110, ui.Auto, 0, ui.Auto).Radius(12).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 12, 32, -6, ui.RGBA(0, 0, 0, 0.28)).
			Padding(10).Gap(8).Children(func() {
			before := model.State.Query
			query := before
			ui.TextInput(c, &query).Placeholder(locales.QuickOpenPlaceholder()).Label(locales.QuickOpenTitle()).
				AutoFocus().OnSubmit(func() { dispatch(Action{Kind: OpenSelection, Index: model.State.Selected}) }).
				OnTextKey(func(k ui.TextKey) bool {
					if k.Mods != ui.Ctrl || (k.Key != ui.KeyN && k.Key != ui.KeyP) || model.Rows == nil || model.Rows.Len() == 0 {
						return false
					}
					step := 1
					if k.Key == ui.KeyP {
						step = -1
					}
					index := max(0, min(model.State.Selected+step, model.Rows.Len()-1))
					dispatch(Action{Kind: MoveSelection, Index: index})
					return true
				})
			if query != before {
				dispatch(Action{Kind: QueryChanged, Query: query})
			}
			if model.Rows == nil || model.Rows.Len() == 0 {
				ui.Text(c, locales.QuickOpenNoMatch()).FontSize(12).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				return
			}
			selected := max(0, min(model.State.Selected, model.Rows.Len()-1))
			limit := min(model.Rows.Len(), 12)
			for index := 0; index < limit; index++ {
				row := model.Rows.At(index)
				rowIndex := index
				item := ui.Row(c).Height(42).Radius(6).Padding(0, 10).Gap(9).
					AlignItems(ui.Center).Cursor(ui.CursorPointer)
				switch {
				case rowIndex == selected:
					if t.Dark {
						item.Background(ui.RGBA(107, 147, 255, 0.30))
					} else {
						item.Background(ui.RGBA(107, 147, 255, 0.20))
					}
				case item.Hovered():
					item.Background(t.SurfaceHover)
				}
				item.Children(func() {
					ui.Icon(c, model.FileIcon).Size(13, 13).TextColor(model.IconTint)
					ui.Column(c).FillWidth().Gap(1).Children(func() {
						highlight(c, row.Title, row.TitleMatches, t.Text, t.Accent, 12.5)
						highlight(c, row.PathText, row.PathMatches, t.TextMuted, t.Accent, 10.5)
					})
				})
				item.OnClick(func() { dispatch(Action{Kind: OpenSelection, Index: rowIndex}) })
			}
			ui.Row(c).Height(18).Padding(0, 2).Children(func() {
				ui.Text(c, locales.QuickOpenCount(model.Rows.Len())).FontSize(10).TextColor(t.TextMuted)
			})
		})
	})
}

func highlight(c *ui.Context, text string, matches []int, color, accent ui.Color, size float32) ui.Element {
	runes := []rune(text)
	if len(matches) == 0 {
		return ui.RichText(c, ui.Span{Text: text}).FontSize(size).TextColor(color).SingleLine().Ellipsis("…")
	}
	spans := make([]ui.Span, 0, len(matches)*2+1)
	start := 0
	for i := 0; i < len(matches); {
		first := matches[i]
		if first > start {
			spans = append(spans, ui.Span{Text: string(runes[start:first])})
		}
		end := first + 1
		i++
		for i < len(matches) && matches[i] == end {
			end++
			i++
		}
		spans = append(spans, ui.Span{Text: string(runes[first:end]), Color: accent, Background: ui.RGBA(accent.R, accent.G, accent.B, 0.18), Radius: 2})
		start = end
	}
	if start < len(runes) {
		spans = append(spans, ui.Span{Text: string(runes[start:])})
	}
	return ui.RichText(c, spans...).FontSize(size).TextColor(color).SingleLine().Ellipsis("…")
}
