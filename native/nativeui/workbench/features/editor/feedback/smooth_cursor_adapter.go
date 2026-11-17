package feedback

import (
	"time"
)

// CursorMove is the value snapshot needed to decide whether a caret change
// should animate. The host resolves keymap and selection state before calling
// the feedback policy.
type CursorMove struct {
	From, To        int
	SelectionAnchor int
	Enabled         bool
	Navigation      bool
	Extending       bool
	ModalNormal     bool
	Now             time.Time
}

// BeginCursorMove applies the movement eligibility policy and records a
// visual transition. Document selection remains owned by the editor host.
func (h *Handle) BeginCursorMove(move CursorMove) bool {
	if h == nil {
		return false
	}
	if !move.Enabled || !move.Navigation || move.Extending || move.From == move.To ||
		move.SelectionAnchor != move.From || !move.ModalNormal {
		h.CancelCursor()
		return false
	}
	now := move.Now
	if now.IsZero() {
		now = time.Now()
	}
	h.BeginCursor(move.From, move.To, now)
	return h.CursorState().Active
}
