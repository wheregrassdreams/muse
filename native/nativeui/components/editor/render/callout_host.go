package editor

import (
	"strings"

	"github.com/egoist/mygo/ui"
	syntax "muse/markdown/syntax"
	render "muse/nativeui/components/editor/render"
)

// quoteContainer preserves parser-owned containers and extends alert headers
// with Obsidian's optional title; source and byte offsets remain untouched.
func quoteContainer(src string, id int, b *syntax.IncrementalBlock) cont {
	c := cont{kind: cQuote, id: id, start: b.Start, alert: b.Alert}
	line := src[b.Start:]
	if n := strings.IndexByte(line, '\n'); n >= 0 {
		line = line[:n]
	}
	line = strings.TrimLeft(line, " >\t")
	if strings.HasPrefix(line, "[!") {
		if n := strings.IndexByte(line, ']'); n >= 0 {
			for a := syntax.IncrementalAlertNote; a <= syntax.IncrementalAlertCaution; a++ {
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
func isCalloutContainer(c cont) bool { return c.kind == cQuote && c.alert != syntax.IncrementalNoAlert }

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
		title := v.c.title
		if v.headerEditing {
			title = ""
		}
		block := render.Block{
			Kind:   render.CalloutBlock,
			Bounds: ui.Rect{X: ox + v.x, Y: oy + v.y, W: e.colW - (v.x - e.colX), H: v.z - v.y + e.lk.size*.6},
			Title:  title, Alert: renderAlertKind(v.c.alert),
		}
		e.renderScratch.addBlock(block)
	}
}

func renderAlertKind(alert syntax.IncrementalAlert) render.AlertKind {
	switch alert {
	case syntax.IncrementalAlertTip:
		return render.AlertTip
	case syntax.IncrementalAlertImportant:
		return render.AlertImportant
	case syntax.IncrementalAlertWarning:
		return render.AlertWarning
	case syntax.IncrementalAlertCaution:
		return render.AlertCaution
	default:
		return render.AlertNote
	}
}

// calloutHeader identifies the source line rather than comparing its text to
// body content. Keeping that line as a leaf makes every header byte addressable.
func (e *Editor) calloutHeader(l *leaf) (cont, int, bool) {
	for i := len(l.conts) - 1; i >= 0; i-- {
		c := l.conts[i]
		if isCalloutContainer(c) && lineStartAt(e.Source(), c.start) == l.lineStart {
			return c, lineEndAt(e.Source(), c.start), true
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
