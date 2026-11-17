// Package commandpalette renders an application-projected command list and
// translates search, selection, execution, and dismissal input into actions.
// Command availability and effects remain with the caller.
package commandpalette

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/search/quickopen"
)

type Command struct {
	ID, Name, Hint string
}

// filter ranks registered commands for the panel query.
func filter(query string, commands []Command) []Command {
	query = strings.TrimSpace(query)
	if query == "" {
		return append([]Command(nil), commands...)
	}
	candidates := make([]quickopen.Candidate, len(commands))
	byID := make(map[string]Command, len(commands))
	for i, command := range commands {
		candidates[i] = quickopen.Candidate{Path: command.ID, PathText: command.Name, Title: command.Name}
		byID[command.ID] = command
	}
	matches := quickopen.Rank(query, candidates)
	filtered := make([]Command, 0, len(matches))
	for _, match := range matches {
		if command, ok := byID[match.Path]; ok {
			filtered = append(filtered, command)
		}
	}
	return filtered
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

// state owns the command palette's transient presentation state. The root
// application supplies command data and handles the resulting use cases.
type state struct {
	open     bool
	query    string
	selected int
}

// Handle keeps transient command-palette state inside this panel package.
// The composition root may route events through it without retaining the
// panel's concrete state object.
type Handle struct{ state *state }

func NewHandle() Handle { return Handle{state: &state{}} }

func (h *Handle) ensureState() *state {
	if h.state == nil {
		h.state = &state{}
	}
	return h.state
}

func (h *Handle) Open()          { h.ensureState().Open() }
func (h *Handle) Close()         { h.ensureState().Close() }
func (h Handle) IsOpen() bool    { return h.state != nil && h.state.IsOpen() }
func (h *Handle) Apply(a Action) { h.ensureState().Apply(a) }
func (h Handle) ViewModel(c []Command) ViewModel {
	if h.state == nil {
		return ViewModel{Commands: filter("", c)}
	}
	return h.state.ViewModel(c)
}

func (s *state) Open() {
	s.open = true
	s.query = ""
	s.selected = 0
}

func (s *state) Close() { s.open = false }

func (s state) IsOpen() bool { return s.open }

// Apply consumes stateful panel actions. Execute and Dismiss are left to the
// composition root because they trigger application workflows.
func (s *state) Apply(action Action) {
	switch action.Kind {
	case QueryChanged:
		s.query = action.Query
		s.selected = 0
	case MoveSelection:
		s.selected = max(0, action.Index)
	}
}

func (s state) ViewModel(commands []Command) ViewModel {
	commands = filter(s.query, commands)
	return ViewModel{Query: s.query, Selected: clamp(s.selected, len(commands)), Commands: commands}
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
