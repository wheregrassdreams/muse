package mdedit

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/mdedit/internal/parser"

	"muse/appearance"
)

// quoteContainer preserves parser-owned containers and extends alert headers
// with Obsidian's optional title; source and byte offsets remain untouched.
func quoteContainer(src string, id int, b *parser.Block) cont {
	c := cont{kind: cQuote, id: id, start: b.Start, alert: b.Alert}
	line := src[b.Start:]
	if n := strings.IndexByte(line, '\n'); n >= 0 {
		line = line[:n]
	}
	line = strings.TrimLeft(line, " >\t")
	if strings.HasPrefix(line, "[!") {
		if n := strings.IndexByte(line, ']'); n >= 0 {
			for a := parser.AlertNote; a <= parser.AlertCaution; a++ {
				if strings.EqualFold(line[2:n], a.String()) {
					c.alert = a
					c.title = strings.TrimSpace(line[n+1:])
					break
				}
			}
		}
	}
	if c.title == "" {
		c.title = c.alert.String()
	}
	return c
}
func isCalloutContainer(c cont) bool { return c.kind == cQuote && c.alert != parser.NoAlert }

func calloutColor(a parser.Alert, accent ui.Color, dark bool) ui.Color {
	switch a {
	case parser.AlertWarning:
		return ui.RGB(202, 135, 12)
	case parser.AlertCaution:
		return ui.RGB(218, 65, 70)
	case parser.AlertTip:
		return ui.RGB(36, 155, 113)
	case parser.AlertImportant:
		return ui.RGB(139, 93, 211)
	default:
		return appearance.From(accent, dark).Accent
	}
}

// paintCallouts must run before quote bars and text. The caller excludes
// isCalloutContainer containers from ordinary quotation bars.
func (e *Editor) paintCallouts(p *ui.Painter, ox, oy float32) {
	type card struct {
		c             cont
		x, y, z       float32
		headerEditing bool
	}
	var cards []*card
	byID := map[int]*card{}
	vis := p.Visible()
	top, bottom := vis.Y-oy-40, vis.Y-oy+vis.H+40
	// Only leaves that can contribute pixels to this frame are needed here.
	// A callout that extends beyond the viewport is clipped by the painter, so
	// its visible portion can be accumulated from the visible leaves alone.
	for _, l := range e.paintVisibleSpan(top, bottom) {
		x := e.colX
		for _, c := range l.conts {
			if isCalloutContainer(c) {
				v := byID[c.id]
				if v == nil {
					v = &card{c: c, x: x, y: l.y - e.lk.size*.45, z: l.y + l.h}
					byID[c.id] = v
					cards = append(cards, v)
				}
				v.z = l.y + l.h
			}
			x += e.indentOf([]cont{c})
		}
		if c, _, ok := e.calloutHeader(l); ok {
			if v := byID[c.id]; v != nil {
				v.headerEditing = e.calloutHeaderSourceOpen(l)
			}
		}
	}
	for _, v := range cards {
		if v.z < top || v.y > bottom {
			continue
		}
		col := calloutColor(v.c.alert, e.lk.accent, e.lk.dark)
		alpha := float32(.1)
		if e.lk.dark {
			alpha = .18
		}
		p.Fill(ui.Rect{X: ox + v.x, Y: oy + v.y, W: e.colW - (v.x - e.colX), H: v.z - v.y + e.lk.size*.6}, col.Alpha(alpha), 8)
		if e.lk.dark {
			switch v.c.alert {
			case parser.AlertWarning:
				col = ui.RGB(240, 185, 75)
			case parser.AlertCaution:
				col = ui.RGB(255, 115, 120)
			case parser.AlertTip:
				col = ui.RGB(80, 205, 155)
			case parser.AlertImportant:
				col = ui.RGB(185, 145, 255)
			}
		}
		if !v.headerEditing {
			paintCalloutIcon(p, ox+v.x+e.lk.size*1.4, oy+v.y+e.lk.size*.5, e.lk.size, col, v.c.alert == parser.AlertWarning || v.c.alert == parser.AlertCaution)
			p.RichText(ox+v.x+e.lk.size*2.8, oy+v.y+e.lk.size*.45, e.colW-(v.x-e.colX)-e.lk.size*4.2, ui.Span{Text: v.c.title, Color: col, Size: e.lk.size, Weight: 600})
		}
	}
}

// calloutHeader identifies the source line rather than comparing its text to
// body content. Keeping that line as a leaf makes every header byte addressable.
func (e *Editor) calloutHeader(l *leaf) (cont, int, bool) {
	for i := len(l.conts) - 1; i >= 0; i-- {
		c := l.conts[i]
		if isCalloutContainer(c) && lineStartAt(e.src, c.start) == l.lineStart {
			return c, lineEndAt(e.src, c.start), true
		}
	}
	return cont{}, 0, false
}

// calloutHeaderSourceOpen is shared by the source projection and semantic
// chrome: the preview title/icon is hidden exactly while the header's source
// line is under the caret or a real source selection.
func (e *Editor) calloutHeaderSourceOpen(l *leaf) bool {
	return e.calloutHeaderSourceOpenAt(l, e.revealCaret())
}

func (e *Editor) calloutHeaderSourceOpenAt(l *leaf, caret int) bool {
	_, end, ok := e.calloutHeader(l)
	if !ok {
		return false
	}
	return caret >= l.lineStart && caret <= end ||
		sourceRangeTargeted(e, l.lineStart, end, false)
}

func (e *Editor) projectCalloutHeader(b *builder, l *leaf, at int) {
	_, end, ok := e.calloutHeader(l)
	if !ok {
		return
	}
	sourceOpen := e.calloutHeaderSourceOpenAt(l, b.reveal)
	old := append([]piece(nil), b.pieces...)
	b.pieces = nil
	b.text.Reset()
	b.u16 = 0
	if sourceOpen {
		b.marker(l.lineStart, end, istyle{})
	} else {
		if b.quotePrefixMode {
			b.prefixSpacer(l.start, b.prefixIndent)
		}
		// Invisible atom retains the line's source extent and hit target.
		b.spanAtom(l.start, end, " ", b.span(istyle{}))
	}
	for _, v := range old {
		a, z := b.base+v.s0, b.base+v.s1
		if z <= end {
			continue
		}
		if a < end {
			if v.kind == pText {
				v.text = v.text[end-a:]
				v.s0 = end - b.base
			} else {
				continue
			}
		}
		b.add(v)
	}
}

func paintCalloutIcon(p *ui.Painter, x, y, s float32, col ui.Color, warning bool) {
	line := func(a, b, c, d float32) {
		p.StrokePolyline([][2]float32{{x + a*s, y + b*s}, {x + c*s, y + d*s}}, 1.6, col)
	}
	if warning {
		line(.5, .08, .94, .88)
		line(.94, .88, .06, .88)
		line(.06, .88, .5, .08)
		line(.5, .36, .5, .59)
		line(.5, .72, .5, .74)
	} else {
		line(.18, .68, .66, .2)
		line(.66, .2, .82, .36)
		line(.82, .36, .34, .84)
		line(.34, .84, .12, .9)
		line(.12, .9, .18, .68)
		line(.58, .28, .74, .44)
	}
}
