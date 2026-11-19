package editor

import (
	"github.com/egoist/mygo/ui"
	editorprojection "github.com/wheregrassdreams/muse-editor/markdown/render"
	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
	render "muse/nativeui/workbench/features/editor/render"
)

// quoteContainer preserves parser-owned containers and extends alert headers
// with Obsidian's optional title; source and byte offsets remain untouched.
func quoteContainer(src string, id int, b syntax.IncrementalBlock) cont {
	projected := editorprojection.SourceQuoteContainer(src, id, b)
	return cont{kind: hostContainerKind(projected.Kind), alert: projected.Alert, title: projected.Title, id: projected.ID, start: projected.Start}
}
func isCalloutContainer(c cont) bool {
	kind := editorprojection.SourceQuote
	if c.kind == cItem {
		kind = editorprojection.SourceItem
	} else if c.kind == cFootnote {
		kind = editorprojection.SourceFootnote
	}
	return editorprojection.IsSourceCalloutContainer(editorprojection.SourceContainer{Kind: kind, Alert: c.alert})
}

// paintCallouts must run before quote bars and text. The caller excludes
// isCalloutContainer containers from ordinary quotation bars.
func (e *Editor) paintCallouts(p *ui.Painter, ox, oy float32) {
	vis := p.Visible()
	top, bottom := vis.Y-oy-40, vis.Y-oy+vis.H+40
	// Only leaves that can contribute pixels to this frame are needed here.
	// A callout that extends beyond the viewport is clipped by the painter, so
	// its visible portion can be accumulated from the visible leaves alone.
	lines := e.calloutLines(e.paintVisibleSpan(top, bottom))
	for _, block := range render.ProjectCallouts(lines, top, bottom) {
		block.Bounds.X += ox
		block.Bounds.Y += oy
		e.renderScratch.addBlock(block)
	}
}

func (e *Editor) calloutLines(visible []*leaf) []render.CalloutLine {
	var lines []render.CalloutLine
	for _, l := range visible {
		x := e.colX
		for _, c := range l.conts {
			if isCalloutContainer(c) {
				header := &leaf{lineStart: lineStartAt(e.Source(), c.start), conts: []cont{c}}
				lines = append(lines, render.CalloutLine{
					ID: c.id, X: x, Y: l.y, Height: l.h, Right: e.colX + e.colW,
					FontSize: e.lk.size, Title: c.title, Alert: renderAlertKind(c.alert),
					HeaderSourceRevealed: e.calloutHeaderSourceOpen(header),
				})
			}
			x += e.indentOf([]cont{c})
		}
	}
	return lines
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
