// Package commandpalette renders an application-projected command list and
// translates search, selection, execution, and dismissal input into actions.
// Command availability and effects remain with the caller.
package commandpalette

import (
	"github.com/egoist/mygo/ui"
	"muse/locales"
)

type Command struct {
	ID, Name, Hint string
}

type ViewModel struct {
	Query    string
	Selected int
	Commands []Command
}

type ActionKind uint8

const (
	QueryChanged ActionKind = iota + 1
	MoveSelection
	Execute
	Dismiss
)

type Action struct {
	Kind  ActionKind
	Query string
	Index int
	ID    string
}

func Render(c *ui.Context, model ViewModel, dispatch func(Action)) {
	if c.Shortcut(0, ui.KeyEscape) {
		dispatchAction(dispatch, Action{Kind: Dismiss})
		return
	}
	selected := clamp(model.Selected, len(model.Commands))
	if c.Shortcut(0, ui.KeyUp) {
		if selected > 0 {
			dispatchAction(dispatch, Action{Kind: MoveSelection, Index: selected - 1})
		}
	}
	if c.Shortcut(0, ui.KeyDown) {
		if selected < len(model.Commands)-1 {
			dispatchAction(dispatch, Action{Kind: MoveSelection, Index: selected + 1})
		}
	}
	t := c.Theme()
	ui.Box(c).Fill().Background(ui.RGBA(0, 0, 0, 0.32)).Children(func() {
		ui.Column(c).Width(560).Margin(110, ui.Auto, 0, ui.Auto).Radius(12).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 12, 32, -6, ui.RGBA(0, 0, 0, 0.28)).
			Padding(10).Gap(8).Children(func() {
			query := model.Query
			ui.TextInput(c, &query).Placeholder(locales.CommandPalettePlaceholder()).Label(locales.CommandPaletteTitle()).
				AutoFocus().OnSubmit(func() { dispatchAction(dispatch, executeSelected(model.Commands, selected)) })
			if query != model.Query {
				dispatchAction(dispatch, Action{Kind: QueryChanged, Query: query})
			}
			if len(model.Commands) == 0 {
				ui.Text(c, locales.CommandPaletteNoMatch()).FontSize(12).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				return
			}
			limit := min(len(model.Commands), 12)
			for i := 0; i < limit; i++ {
				command := model.Commands[i]
				i := i
				row := ui.Row(c).Height(30).Radius(6).Padding(0, 10).Gap(8).
					AlignItems(ui.Center).Cursor(ui.CursorPointer)
				switch {
				case i == selected:
					if t.Dark {
						row.Background(ui.RGBA(107, 147, 255, 0.30))
					} else {
						row.Background(ui.RGBA(107, 147, 255, 0.20))
					}
				case row.Hovered():
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					ui.Text(c, command.Name).FontSize(12.5).TextColor(t.Text).SingleLine().Ellipsis("…")
					if command.Hint != "" {
						ui.Spacer(c)
						ui.Text(c, command.Hint).FontSize(10.5).TextColor(t.TextMuted)
					}
				})
				row.OnClick(func() { dispatchAction(dispatch, Action{Kind: Execute, ID: command.ID}) })
			}
			ui.Row(c).Height(18).Padding(0, 2).Children(func() {
				ui.Text(c, locales.CommandPaletteCount(len(model.Commands))).FontSize(10).TextColor(t.TextMuted)
			})
		})
	})
}

func executeSelected(commands []Command, selected int) Action {
	if len(commands) == 0 {
		return Action{Kind: Dismiss}
	}
	return Action{Kind: Execute, ID: commands[clamp(selected, len(commands))].ID}
}

func clamp(index, count int) int {
	if count == 0 {
		return 0
	}
	return max(0, min(index, count-1))
}

func dispatchAction(dispatch func(Action), action Action) {
	if dispatch != nil {
		dispatch(action)
	}
}
