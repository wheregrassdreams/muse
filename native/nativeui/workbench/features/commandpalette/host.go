package commandpalette

import "github.com/egoist/mygo/ui"

// Host owns the command palette's transient interaction state and the command
// candidates supplied by the composition root. It projects candidates for the
// view and returns typed intent; it never owns command availability or effects.
type Host struct {
	state    Handle
	commands []Command
}

func NewHost() *Host { return &Host{state: NewHandle()} }

// Open replaces the candidate snapshot and opens a fresh query session.
func (h *Host) Open(commands []Command) {
	h.SetCommands(commands)
	h.state.Open()
}

// SetCommands refreshes candidates without changing query or selection state.
func (h *Host) SetCommands(commands []Command) {
	h.ensureState()
	h.commands = append(h.commands[:0], commands...)
}

func (h *Host) Close() {
	h.ensureState()
	h.state.Close()
}

func (h *Host) IsOpen() bool { return h != nil && h.state.IsOpen() }

// ViewModel returns the current filtered and selected command projection.
func (h *Host) ViewModel() ViewModel {
	if h == nil {
		return ViewModel{}
	}
	return h.state.ViewModel(h.commands)
}

// Apply consumes presentation actions and returns only intents that the root
// must route. Query and selection changes remain inside this feature. Execute
// is accepted only for a currently visible candidate and carries its stable ID.
func (h *Host) Apply(action Action) Action {
	if h == nil {
		return Action{}
	}
	switch action.Kind {
	case QueryChanged, MoveSelection:
		h.state.Apply(action)
		return Action{}
	case Dismiss:
		h.Close()
		return Action{Kind: Dismiss}
	case Execute:
		for _, command := range h.ViewModel().Commands {
			if action.ID != "" && command.ID == action.ID {
				h.Close()
				return Action{Kind: Execute, ID: command.ID}
			}
		}
	}
	return Action{}
}

// Render renders the current projection and dispatches typed root intents when
// the user selects a command or dismisses the palette. The callback carries no
// app-specific behavior and may only route the resulting action.
func (h *Host) Render(c *ui.Context, dispatch func(Action)) {
	if h == nil || !h.IsOpen() {
		return
	}
	Render(c, h.ViewModel(), func(action Action) {
		if routed := h.Apply(action); routed.Kind != 0 {
			if dispatch != nil {
				dispatch(routed)
			}
		}
	})
}

func (h *Host) ensureState() {
	if h.state.state == nil {
		h.state = NewHandle()
	}
}
