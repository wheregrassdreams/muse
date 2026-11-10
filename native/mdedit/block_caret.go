package mdedit

import (
	"math"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
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

// blockCaretTextColor chooses black or white for a readable glyph on the
// solid caret background.
func blockCaretTextColor(bg ui.Color) ui.Color {
	linear := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	l := 0.2126*linear(bg.R) + 0.7152*linear(bg.G) + 0.0722*linear(bg.B)
	if 1.05/(l+0.05) >= (l+0.05)/0.05 {
		return ui.RGB(255, 255, 255)
	}
	return ui.RGB(0, 0, 0)
}

// blockCaretGlyph returns the displayed glyph at a UTF-16 display offset,
// preserving its font and decorations while replacing its foreground and
// removing backgrounds that would cover the solid caret.
func blockCaretGlyph(d display, at int, fg ui.Color) (ui.Span, bool) {
	for _, pc := range d.pieces {
		if at < pc.d0 || at >= pc.d1 || pc.text == "" {
			continue
		}
		off := at - pc.d0
		for _, r := range pc.text {
			n := 1
			if r > 0xffff {
				n = 2
			}
			if off < n {
				if r == '\ufffc' {
					return ui.Span{}, false
				}
				sp := pc.span
				sp.Text = string(r)
				sp.Color = fg
				sp.Background = ui.Color{}
				sp.Radius, sp.PaddingX, sp.PaddingY = 0, 0, 0
				return sp, true
			}
			off -= n
		}
	}
	return ui.Span{}, false
}
