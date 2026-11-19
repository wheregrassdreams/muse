package focus

import (
	"errors"

	commands "muse/nativeui/workbench/framework/interactions/commands"
)

var (
	ErrEmptyPanelOrder = errors.New("focus panel order must not be empty")
	ErrEmptyPanelID    = errors.New("focus panel ID must not be empty")
	ErrDuplicatePanel  = errors.New("duplicate panel ID in focus order")
	ErrUnknownPanel    = errors.New("panel is not in focus order")
)

// FocusRouter stores the logical panel order and current panel without
// depending on toolkit focus handles. The composition root performs the
// native focus transition after the router returns a target.
type FocusRouter struct {
	order   []commands.PanelID
	indices map[commands.PanelID]int
	current commands.PanelID
}

func NewFocusRouter(order []commands.PanelID, initial commands.PanelID) (*FocusRouter, error) {
	if len(order) == 0 {
		return nil, ErrEmptyPanelOrder
	}
	r := &FocusRouter{order: append([]commands.PanelID(nil), order...), indices: make(map[commands.PanelID]int, len(order))}
	for i, id := range r.order {
		if id == "" {
			return nil, ErrEmptyPanelID
		}
		if _, exists := r.indices[id]; exists {
			return nil, ErrDuplicatePanel
		}
		r.indices[id] = i
	}
	if initial == "" {
		initial = r.order[0]
	}
	if _, exists := r.indices[initial]; !exists {
		return nil, ErrUnknownPanel
	}
	r.current = initial
	return r, nil
}

func (r *FocusRouter) Current() commands.PanelID {
	if r == nil {
		return ""
	}
	return r.current
}

// Order returns an independent copy of the routing order.
func (r *FocusRouter) Order() []commands.PanelID {
	if r == nil {
		return nil
	}
	return append([]commands.PanelID(nil), r.order...)
}

// Focus changes the logical target when panel is in the configured order.
// It reports whether the target is valid, including when it was already
// current.
func (r *FocusRouter) Focus(panel commands.PanelID) bool {
	if r == nil {
		return false
	}
	if _, exists := r.indices[panel]; !exists {
		return false
	}
	r.current = panel
	return true
}

// Move moves one adjacent step without wrapping, matching directional focus
// movement. Direction less than zero moves backward; greater than zero moves
// forward. A zero direction or an edge returns the current panel and false.
func (r *FocusRouter) Move(direction int) (commands.PanelID, bool) {
	if r == nil || direction == 0 {
		return r.Current(), false
	}
	index, ok := r.indices[r.current]
	if !ok {
		return r.current, false
	}
	step := 1
	if direction < 0 {
		step = -1
	}
	target := index + step
	if target < 0 || target >= len(r.order) {
		return r.current, false
	}
	r.current = r.order[target]
	return r.current, true
}

// Cycle wraps through the configured panels, matching the keyboard cycle
// shortcut. A single-panel order remains unchanged and reports false.
func (r *FocusRouter) Cycle(reverse bool) (commands.PanelID, bool) {
	if r == nil || len(r.order) < 2 {
		return r.Current(), false
	}
	index, ok := r.indices[r.current]
	if !ok {
		return r.current, false
	}
	step := 1
	if reverse {
		step = -1
	}
	index = (index + step + len(r.order)) % len(r.order)
	r.current = r.order[index]
	return r.current, true
}
