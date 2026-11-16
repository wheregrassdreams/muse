package editor

import (
	"time"

	"muse/editor/input/vim"
)

// beginSmoothCursorMove records visual transition intent in the feedback
// state. The document caret and selection still move synchronously.
func (e *Editor) beginSmoothCursorMove(from, to int, extend bool) {
	e.beginSmoothCursorMoveAt(from, to, extend, time.Now())
}

func (e *Editor) beginSmoothCursorMoveAt(from, to int, extend bool, now time.Time) {
	if !e.opts.SmoothCursor || !e.smoothCursorNavigation || extend || from == to || e.Session.Selection().Anchor != from {
		e.feedbackState.CancelCursor()
		return
	}
	mode := e.vimController().Mode()
	if (e.Keymap() != "vim" && e.Keymap() != "helix") || mode != vim.ModeNormal {
		e.feedbackState.CancelCursor()
		return
	}
	e.feedbackState.BeginCursor(from, to, now)
}
