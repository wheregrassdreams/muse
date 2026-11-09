package mdedit

import (
	"github.com/egoist/mygo/ui"
	"unicode/utf8"
)

func (e *Editor) blockCaret() bool { return e.Keymap() != "default" && !e.ModalAcceptsText() }

// Measure the actual shaped character; a line end uses a visible empty cell.
func (e *Editor) blockCaretRect(r ui.Rect) ui.Rect {
	r.W = max(e.lk.size*0.6, 2)
	if e.caret < len(e.src) && e.src[e.caret] != '\n' {
		_, n := utf8.DecodeRuneInString(e.src[e.caret:])
		if next, ok := e.caretRect(e.caret + n); ok && next.Y-r.Y < 1 && next.Y-r.Y > -1 && next.X > r.X {
			r.W = next.X - r.X
		}
	}
	return r
}
