package mdedit

import (
	"math"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

func (e *Editor) blockCaret() bool { return e.Keymap() != "default" && !e.ModalAcceptsText() }

// vimNormalOffset keeps a normal-mode caret on a character. Vim's normal
// cursor identifies a character cell, while the native editor stores the
// boundary immediately before it.
func (e *Editor) vimNormalOffset(off int) int {
	off = max(0, min(off, len(e.src)))
	// Structural lines own their source boundary even when a Markdown
	// container marker (for example the `>` on a blank quote line) occupies
	// the physical source line. Clamping that boundary as though it were a
	// text line pulls vertical movement back into the preceding paragraph.
	if u := e.unitAt(off); u != nil && u.isLine() {
		return off
	}
	start, end := e.sourceLineRange(off)
	if off == end && end > start {
		// A folded Markdown soft break is drawn as a space in one visual line.
		// Its source newline boundary remains a valid horizontal caret stop;
		// treating it as a physical Vim EOL makes `l` jump back to the previous
		// visible character and stalls across the folded line break.
		if e.vimFoldedSoftBreakAt(off) {
			return off
		}
		return e.normalize(e.previousGraphemeBoundary(end), -1)
	}
	// Keep an existing legal navigation stop exactly where it was. Applying
	// nearest-stop normalization here would pull motions back across atoms
	// such as inline math and make Visual movement appear stuck.
	return off
}

func (e *Editor) vimFoldedSoftBreakAt(off int) bool {
	if off < 0 || off >= len(e.src) || e.src[off] != '\n' {
		return false
	}
	u := e.unitAt(off)
	if u == nil {
		return false
	}
	v := e.unitView(u, off)
	if v == nil {
		return false
	}
	rel := off - u.lineStart
	for _, p := range v.disp.pieces {
		if p.kind == pAtom && p.s0 == rel && p.s1 > p.s0 && p.text == " " {
			return true
		}
	}
	return false
}

// vimPointerOffset converts the insertion boundary hit by the native text
// layout to the character cell under the pointer. In particular, a hit on
// the right half of a glyph returns the boundary after it; Vim needs that
// glyph's starting boundary instead.
func (e *Editor) vimPointerOffset(off int, x, y float32) int {
	if _, _, _, boundary := e.tableBoundaryAtOffset(off); boundary {
		return off
	}
	if table, _ := e.tableAtEdge(off); table != nil {
		return off
	}
	off = e.normalize(off, 1)
	best, bestDistance := off, math.Inf(1)
	consider := func(start, end int) {
		if end <= start {
			return
		}
		a, aok := e.caretRect(start)
		z, zok := e.caretRect(end)
		if !aok || !zok || math.Abs(float64(a.Y-z.Y)) >= 1 {
			return
		}
		centerX := (a.X + z.X) / 2
		centerY := a.Y + a.H/2
		// TextLayout.IndexAt returns the nearest insertion boundary. Compare
		// both adjacent glyph centers so left and right halves map symmetrically.
		dx, dy := float64(centerX-x), float64(centerY-y)
		distance := dx*dx + dy*dy*4
		if distance < bestDistance {
			best, bestDistance = start, distance
		}
	}
	consider(e.step(off, -1, false), off)
	consider(off, e.step(off, 1, false))
	if math.IsInf(bestDistance, 1) {
		best = off
	}
	return e.vimNormalOffset(best)
}

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
