package mdedit

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// The navigation rail marks every heading of the document with a tick
// along the right edge of the editor, as a map of the headings, however
// long the document. The ticks sit as one centered group, and the ones
// near the heading the reader is in, or the one the pointer is on, grow
// longer than the others: a lens that shows where the reader is and where
// the pointer goes. The pointer previews a heading's text and the first
// lines of its section, and a click brings the heading to the middle of
// the view.
const (
	// railWidth is the width of the rail's strip, and railInset how far it
	// stands from the editor's edges. The ticks take railTickWidth inside,
	// at the strip's right end, and railItemHeight is the room of each
	// row, unless there are too many headings for them all to fit.
	railWidth      = float32(28)
	railInset      = float32(12)
	railTickWidth  = float32(16)
	railTickHeight = float32(2)
	railItemHeight = float32(14)
	railCardWidth  = float32(256)
	// railEdge is how near the document's start or end the view must be
	// for the first or last heading to be the current one, rather than the
	// one at the middle of the view.
	railEdge = float32(56)
)

// railScale returns how wide the tick of a heading is when the highlighted
// one is dist ticks away: the lens falls off by distance, the same at
// every count.
func railScale(dist int) float32 {
	switch dist {
	case 0:
		return 1
	case 1:
		return 0.68
	case 2:
		return 0.44
	}
	return 0.25
}

// railView draws the rail over the right edge of the scroll area b, and
// takes the pointer over it.
func (e *Editor) railView(c *ui.Context, b ui.Rect) {
	e.railAnim = false
	if !e.railVisible(b) {
		e.railHover, e.railCardOn = -1, false
		e.railRect = ui.Rect{}
		return
	}
	avail := b.H - 2*railInset
	e.railYs = e.railPositions(avail)
	e.railActive = e.railCurrent()
	// The widths and the preview's place move from frame to frame toward
	// where the pointer puts them: mygo has no CSS transitions, so the
	// frames do that work here.
	if n := len(e.railLeaves); len(e.railScales) != n {
		e.railScales = make([]float32, n)
		for i := range e.railScales {
			e.railScales[i] = 1
		}
	}
	hi := e.railHover
	if hi < 0 {
		hi = e.railActive
	}
	for i := range e.railScales {
		dist := 1 << 20
		if hi >= 0 {
			dist = max(i-hi, hi-i)
		}
		e.railStep(&e.railScales[i], railScale(dist))
	}

	strip := ui.Rect{X: b.X + b.W - railWidth - railInset, Y: b.Y + railInset, W: railWidth, H: avail}
	e.railRect = strip
	ui.Overlay(c, func() {
		rail := ui.Box(c).Absolute().Left(strip.X).Top(strip.Y).Width(strip.W).Height(strip.H).
			Label("标题导航").Cursor(ui.CursorPointer).Draw(e.paintRail)
		// The pointer's moves over the rail ask for frames (PointerPosition),
		// but its leaving does not: View asks for one while a tick is
		// hovered, so that the preview goes when the pointer does.
		_, py, over := rail.PointerPosition()
		hover := -1
		if over {
			hover = e.railIndexAt(py)
		}
		if hover != e.railHover {
			e.railHover = hover
			if hover < 0 {
				e.railCardOn = false
			}
			e.invalidate()
		}
		if hover >= 0 && rail.Clicked() {
			e.railJump(hover)
		}
		if e.railHover >= 0 && e.railHover < len(e.railYs) {
			center := strip.Y + e.railYs[e.railHover]
			if !e.railCardOn {
				// The card is a singleton: it comes from the rail's middle
				// the first time a tick is hovered.
				e.railCardOn = true
				e.railCardY = strip.Y + strip.H/2
			}
			e.railStep(&e.railCardY, center)
			e.railPreview(c, b)
		}
	})
}

// railVisible reports whether the rail has something to show: more than
// one heading, in a document that scrolls.
func (e *Editor) railVisible(b ui.Rect) bool {
	return len(e.railLeaves) > 1 && e.docH > e.viewH+1 &&
		b.W > railWidth+2*railInset && b.H > 2*railInset+railItemHeight
}

// railPositions returns the center of every tick along a rail of height h:
// one group, centered in it, a row of railItemHeight each until the rows
// no longer fit, when they share the height evenly.
func (e *Editor) railPositions(h float32) []float32 {
	n := len(e.railLeaves)
	e.railYs = e.railYs[:0]
	if n == 0 || h <= 0 {
		e.railPitch = 0
		return nil
	}
	pitch := min(railItemHeight, h/float32(n))
	e.railPitch = pitch
	top := (h - pitch*float32(n)) / 2
	for i := 0; i < n; i++ {
		e.railYs = append(e.railYs, top+(float32(i)+0.5)*pitch)
	}
	return e.railYs
}

// railIndexAt returns the tick whose row holds y, a y relative to the
// rail's top, or -1 above or below the rows.
func (e *Editor) railIndexAt(y float32) int {
	n := len(e.railYs)
	if n == 0 || e.railPitch <= 0 {
		return -1
	}
	top := e.railYs[0] - e.railPitch/2
	if y < top-2 || y > top+e.railPitch*float32(n)+2 {
		return -1
	}
	i := int((y - top) / e.railPitch)
	return min(max(i, 0), n-1)
}

// railCurrent returns the heading whose section holds the middle of the
// view, the first at the document's start and the last at its end, or -1
// with no headings.
func (e *Editor) railCurrent() int {
	n := len(e.railLeaves)
	if n == 0 {
		return -1
	}
	if e.scroll.Y <= railEdge {
		return 0
	}
	if maxOff := e.docH - e.viewH; maxOff > 0 && e.scroll.Y >= maxOff-railEdge {
		return n - 1
	}
	line := e.scroll.Y + e.viewH/2
	cur := 0
	for i, l := range e.railLeaves {
		if l.y > line {
			break
		}
		cur = i
	}
	return cur
}

// railJump brings heading i to the middle of the view, and the caret
// there.
func (e *Editor) railJump(i int) {
	if i < 0 || i >= len(e.railLeaves) {
		return
	}
	e.scrollToOffsetCentered(e.railLeaves[i].start)
	e.Focus()
}

// railStep moves v a step toward target, and reports whether it has
// arrived: the easing of the lens and of the card.
func (e *Editor) railStep(v *float32, target float32) {
	d := target - *v
	if d > -0.01 && d < 0.01 {
		*v = target
		return
	}
	*v += d * 0.22
	e.railAnim = true
}

// railHeadingText returns the text of heading i, as the preview shows it.
func (e *Editor) railHeadingText(i int) string {
	e.railPreviewText(i)
	return e.railText
}

// railHeadingDesc returns the first lines of heading i's section, as the
// preview shows them.
func (e *Editor) railHeadingDesc(i int) string {
	e.railPreviewText(i)
	return e.railDesc
}

// railPreviewText reads the text and the section's first lines of heading
// i, once per revision.
func (e *Editor) railPreviewText(i int) {
	if i < 0 || i >= len(e.railLeaves) {
		e.railText, e.railDesc = "", ""
		return
	}
	if e.railTextIdx == i && e.railTextRev == e.rev {
		return
	}
	l := e.railLeaves[i]
	e.railText = strings.TrimSpace(e.plainText(e.p.doc.Inlines(l.id)))
	e.railDesc = ""
	if i < len(e.railNext) && e.railNext[i] != nil && e.railNext[i].id >= 0 {
		e.railDesc = railExcerpt(e.plainText(e.p.doc.Inlines(e.railNext[i].id)))
	}
	e.railTextIdx, e.railTextRev = i, e.rev
}

// railExcerpt returns text as a short line of the preview: its whitespace
// runs made single spaces, and its end cut off with an ellipsis.
func railExcerpt(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if r := []rune(text); len(r) > 140 {
		return strings.TrimSpace(string(r[:140])) + "…"
	}
	return text
}

// paintRail draws the ticks: their width tells their distance from the
// highlighted one, and the color whether it is the one the view is in.
func (e *Editor) paintRail(p *ui.Painter, r ui.Rect) {
	e.railRect = r
	for i, y := range e.railYs {
		if i >= len(e.railLeaves) || i >= len(e.railScales) {
			break
		}
		w := railTickWidth * e.railScales[i]
		col := e.lk.marker
		switch {
		case i == e.railActive:
			col = e.lk.text
		case i == e.railHover:
			col = e.lk.muted
		}
		p.Fill(ui.Rect{X: r.X + r.W - w, Y: r.Y + y - railTickHeight/2, W: w, H: railTickHeight}, col, railTickHeight/2)
	}
}

// railPreview floats the hovered heading's text and section beside the
// rail, level first, centered on the tick it belongs to.
func (e *Editor) railPreview(c *ui.Context, b ui.Rect) {
	i := e.railHover
	if i < 0 || i >= len(e.railLeaves) {
		return
	}
	text := e.railHeadingText(i)
	if text == "" {
		return
	}
	t := c.Theme()
	right := e.railRect.X - 8
	left := max(right-min(railCardWidth, b.W-16), b.X+4)
	const cardH = float32(66)
	top := min(max(e.railCardY-cardH/2, b.Y+6), max(b.Y+b.H-6-cardH, b.Y+6))
	shade := ui.RGBA(0, 0, 0, 0.12)
	if t.Dark {
		shade = ui.RGBA(0, 0, 0, 0.4)
	}
	card := ui.Box(c).Absolute().Left(left).Top(top).Width(right-left).Radius(10).
		Padding(8, 12).Background(t.Surface).Border(1, t.Border).
		Shadow(0, 6, 18, 0, shade)
	card.Children(func() {
		ui.Column(c).GapY(2).Children(func() {
			ui.Textf(c, "H%d · %s", e.railLeaves[i].level, text).FontSize(14).FontWeight(600).
				TextColor(t.Text).SingleLine().Ellipsis("…")
			if desc := e.railHeadingDesc(i); desc != "" {
				ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted).MaxLines(2)
			}
		})
	})
}
