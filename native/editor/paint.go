package editor

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	syntax "muse/markdown/syntax"

	"muse/locales"
	"muse/theme"
)

const blink = 530 * time.Millisecond

func missingImageMessage(src string) string {
	src = strings.TrimSpace(src)
	if src == "" || strings.HasPrefix(strings.ToLower(src), "data:") {
		return locales.DocumentMissingImage()
	}
	return locales.DocumentMissingImageNamed(strings.ReplaceAll(src, "\\", "/"))
}

// paint draws the document: the leaves in view, the selection, what a
// search found, and the caret.
func (e *Editor) paint(p *ui.Painter, r ui.Rect) {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	// An app edit may have landed after View built this element. Lay out the
	// current source/parser pair before reading leaves retained by this draw.
	e.layoutIfNeeded()
	e.imageHits = e.imageHits[:0]
	e.elemRect = r
	if r.W != e.elemW && r.W > 0 {
		// The width changed since the layout: lay out again next frame.
		e.elemW = r.W
		e.invalidate()
	}
	t := p.Theme()
	vis := p.Visible()
	top, bottom := vis.Y-r.Y-40, vis.Y-r.Y+vis.H+40
	visible := e.paintVisibleSpan(top, bottom)
	ox, oy := r.X, r.Y
	e.paintCallouts(p, ox, oy)

	// The containers' chrome: quotation bars from their first leaf to their
	// last.
	type bar struct{ x, y0, y1 float32 }
	bars := map[int]*bar{}
	var order []int
	for _, l := range visible {
		x := e.colX
		for _, c := range l.conts {
			if c.kind == cQuote && !isCalloutContainer(c) {
				b := bars[c.id]
				if b == nil {
					b = &bar{x: x, y0: l.y, y1: l.y + l.h}
					bars[c.id] = b
					order = append(order, c.id)
				}
				b.y1 = l.y + l.h
			}
			x += e.indentOf([]cont{c})
		}
	}
	for _, id := range order {
		b := bars[id]
		if b.y1 < top || b.y0 > bottom {
			continue
		}
		p.Fill(ui.Rect{X: ox + b.x + 1, Y: oy + b.y0 - 2, W: 2, H: b.y1 - b.y0 + 4}, e.lk.quoteBar, 1)
	}

	for _, l := range visible {
		e.paintMarkers(p, l, ox, oy)
		drawBlock := func() {
			switch l.kind {
			case kTable:
				e.paintTable(p, l, ox, oy, t)
				for _, row := range l.tab.rows {
					for _, cell := range row {
						e.paintUnit(p, cell, ox, oy)
					}
				}
			default:
				e.paintUnit(p, l, ox, oy)
				if l.kind == kHeading {
					e.paintFoldChevron(p, l, ox, oy)
				}
			}
		}
		if blockContentWidth(l) > blockViewport(l).W {
			clip := blockViewport(l)
			clip.X, clip.Y = clip.X+ox, clip.Y+oy
			p.Clip(clip, 0, drawBlock)
		} else {
			drawBlock()
		}
		e.paintBlockHorizontalBar(p, l, ox, oy)
	}
	for _, l := range visible {
		if l.y+l.h < top || l.y > bottom {
			continue
		}
		if !e.gridBlock(l) || !e.showsChip(l) {
			continue
		}
		e.paintSourceChip(p, l, ox, oy)
	}
	for _, hit := range e.imageHits {
		if !hit.box.Contains(e.pointerX, e.pointerY) && (e.imageDrag == nil || e.imageDrag.hit.obj.sourceFrom != hit.obj.sourceFrom || e.imageDrag.hit.obj.sourceTo != hit.obj.sourceTo) {
			continue
		}
		e.paintImageTools(p, hit, ox, oy)
	}
	if e.src == "" && e.opts.Placeholder != "" && e.compose == "" {
		// The placeholder of an empty note stands on the first line of the
		// document, where the note begins — not wherever the caret's line
		// is: it marks the empty note, and it does not move.
		x, w := e.colX, e.colW
		if len(e.leaves) > 0 {
			x, w = e.leaves[0].x, e.leaves[0].w
		}
		sp := ui.Span{Text: e.opts.Placeholder, Color: e.lk.muted.Alpha(0.6), Size: e.lk.size}
		p.RichText(ox+x, oy+e.padTop+e.lk.size*e.lk.lh/2-e.lk.size*0.7, w, sp)
	}
	if !e.smoothCursorOverlayEnabled() {
		e.paintCaretLayer(p, ox, oy)
	}
}

// paintVisibleSpan finds the laid out leaves that intersect the painter's
// viewport plus its small paint margin. layoutDoc keeps this slice ordered by
// y and excludes folded leaves, whose source-order sentinel y is not sorted.
func (e *Editor) paintVisibleSpan(top, bottom float32) []*leaf {
	leaves := e.visibleLeaves
	yAt := func(i int) float32 {
		if e.virtualIndex.Ready(len(leaves)) {
			return e.virtualY(i)
		}
		return leaves[i].y
	}
	start := sort.Search(len(leaves), func(i int) bool {
		return yAt(i)+leaves[i].h >= top
	})
	end := sort.Search(len(leaves), func(i int) bool {
		return yAt(i) > bottom
	})
	if start >= end {
		return nil
	}
	if e.virtualIndex.Ready(len(leaves)) {
		for i := start; i < end; i++ {
			leaves[i].y = yAt(i)
		}
	}
	return leaves[start:end]
}

func (e *Editor) smoothCursorOverlayEnabled() bool {
	return e.opts.SmoothCursor && (e.Keymap() == "vim" || e.Keymap() == "helix") && e.modal.mode == "Normal"
}

// paintCaretLayer is a separate retained scene element when smooth cursor is
// enabled, so Painter.After redraws only this small overlay instead of every
// document leaf.
func (e *Editor) paintCaretLayer(p *ui.Painter, ox, oy float32) {
	if l := e.horizontalOwner(e.selectionFocus()); l != nil && blockContentWidth(l) > blockViewport(l).W {
		clip := blockViewport(l)
		clip.X, clip.Y, clip.W = clip.X+ox-2, clip.Y+oy, clip.W+4
		p.Clip(clip, 0, func() { e.paintCaret(p, ox, oy) })
	} else {
		e.paintCaret(p, ox, oy)
	}
}

// paintUnit draws a leaf or a cell: its panel, what is found and selected
// in it, its text and its objects.
func (e *Editor) paintUnit(p *ui.Painter, u *leaf, ox, oy float32) {
	a, z := e.textSelectionForUnit(u)
	yankOpacity, yankActive := e.vimYankFlashOpacity(p.Now())
	v := u.view
	if v == nil {
		e.paintBlockRowRect(p, u, nil, ox, oy)
		if yankActive && e.yankFlashTouches(u) {
			box := ui.Rect{X: ox + u.x, Y: oy + u.y, W: u.w, H: u.h}
			p.Fill(box, e.lk.accent.Alpha(0.32*yankOpacity), 3)
		}
		// A blank line has no text and no view: nothing to draw — but a
		// selection across it covers the line, and shows as covering it.
		if e.selectionCoversBlankLine(u, a, z) {
			p.Fill(ui.Rect{X: ox + u.x, Y: oy + u.y, W: u.w, H: u.h}, e.lk.selection, 2)
		}
		return
	}
	x, y := ox+u.x, oy+u.y
	contentX := x - blockScrollOffset(u)
	box := ui.Rect{X: x, Y: y, W: u.w, H: u.h}
	if v.panel {
		p.Fill(box, e.lk.panel, 6)
	}
	e.paintBlockRowRect(p, u, v, ox, oy)
	if v.layout != nil && len(v.disp.codeBoxes) > 0 {
		var ranges []Range
		if e.block != nil {
			ranges = e.SelectionRanges()
		} else if a != z {
			ranges = []Range{{Start: a, End: z}}
		}
		e.paintInlineCodeBoxes(p, u, v, contentX+v.tx, y+v.ty, ranges)
	}
	if u.kind == kRule && v.layout == nil {
		p.Fill(ui.Rect{X: x, Y: y + u.h/2 - 0.75, W: u.w, H: 1.5}, e.lk.border.Alpha(1), 0)
	}
	if v.layout != nil {
		lx, ly := contentX+v.tx, y+v.ty
		highlights, current := e.highlights, e.current
		if e.vimHighlightsActive {
			highlights, current = e.vimHighlights, e.vimCurrent
		}
		for i, h := range highlights {
			col := ui.RGBA(250, 204, 21, 0.35)
			if i == current {
				pulse, _ := e.searchMatchPulse(p.Now())
				col = ui.RGBA(250, 150, 20, 0.55+0.12*pulse)
				e.fillRangePulsed(p, u, v, h.Start, h.End, lx, ly, col, pulse)
				continue
			}
			e.fillRange(p, u, v, h.Start, h.End, lx, ly, col)
		}
		structural := e.tableSelection != nil && u.table != nil && e.tableSelection.lineStart == u.table.lineStart
		if a != z && !structural && e.block == nil {
			sel := e.lk.selection
			if !e.focused {
				sel = sel.Alpha(0.6)
			}
			e.fillRange(p, u, v, a, z, lx, ly, sel)
		}
		if v.diagram == nil {
			v.layout.Draw(p, lx, ly, e.lk.text)
		}
		e.paintObjects(p, u, v, lx, ly)
	} else if a != z && a <= leafLo(e, u) && z >= u.end && !(e.tableSelection != nil && u.table != nil && e.tableSelection.lineStart == u.table.lineStart) {
		p.Fill(box, e.lk.selection, 4)
	}
	if v.block != nil {
		r := ui.Rect{X: contentX + v.bx, Y: y + v.by, W: v.block.w, H: v.block.h}
		e.paintObject(p, v.block, r)
		if v.block.imagePath != "" {
			e.imageHits = append(e.imageHits, imageHit{obj: v.block, box: ui.Rect{X: r.X - e.elemRect.X, Y: r.Y - e.elemRect.Y, W: r.W, H: r.H}, leaf: u})
		}
	}
	if v.diagram != nil {
		e.paintDiagram(p, u, v, ox, oy)
	}
	if yankActive {
		e.paintVimYankFlash(p, u, v, contentX+v.tx, y+v.ty, box, yankOpacity)
	}
	if e.vimImageRow != nil && e.vimImageRow.lineStart == u.lineStart && a < z {
		lineStart, lineEnd := e.sourceLineRange(u.lineStart)
		if a <= lineEnd && z > lineStart {
			if image, _, ok := e.vimImageGeometry(u); ok {
				p.Fill(ui.Rect{X: ox + image.X, Y: oy + image.Y, W: image.W, H: image.H}, e.lk.selection.Alpha(0.28), 4)
			}
		}
	}
	if v.label != "" && !v.reveal && v.panel {
		size := e.lk.size
		sp := ui.Span{Text: v.label, Size: size * 0.7, Color: e.lk.muted.Alpha(0.75)}
		w, _ := p.MeasureText(0, sp)
		lx := x + u.w - w - size*0.6
		p.RichText(lx, y+size*0.35, 0, sp)
		if isMermaid(u) {
			e.paintDiagramToggle(p, u, lx, y)
		}
	}
}

func (e *Editor) yankFlashTouches(u *leaf) bool {
	lo, hi := leafLo(e, u), u.end
	for _, r := range e.vimYankRanges {
		if r.Start < hi && r.End > lo {
			return true
		}
	}
	return false
}

func (e *Editor) paintVimYankFlash(p *ui.Painter, u *leaf, v *leafView, lx, ly float32, box ui.Rect, opacity float32) {
	col := e.lk.accent.Alpha(0.32 * opacity)
	for _, r := range e.vimYankRanges {
		if r.Start >= u.end || r.End <= leafLo(e, u) {
			continue
		}
		if v.layout != nil {
			e.fillRange(p, u, v, r.Start, r.End, lx, ly, col)
		} else {
			p.Fill(box, col, 3)
		}
	}
}

func (e *Editor) selectionCoversBlankLine(u *leaf, a, z int) bool {
	if u == nil || a == z {
		return false
	}
	start, end := e.sourceLineRange(u.start)
	if end < len(e.src) {
		end++ // The linewise range owns the line break ending this blank line.
	}
	return start < end && a <= start && z >= end
}

func (e *Editor) paintBlockRowRect(p *ui.Painter, u *leaf, v *leafView, ox, oy float32) {
	b := e.block
	if b == nil || u == nil || u.table != nil || u.kind == kTable {
		return
	}
	col := e.lk.selection
	if !e.focused {
		col = col.Alpha(0.6)
	}
	fill := func(y, h float32) {
		if y+h/2+0.1 < b.top || y+h/2-0.1 > b.bottom {
			return
		}
		p.Fill(ui.Rect{X: ox + b.left, Y: oy + y, W: max(b.right-b.left, 1), H: max(h, 1)}, col, 2)
	}
	if v == nil || v.layout == nil {
		fill(u.y, u.h)
		return
	}
	for _, line := range v.layout.Lines() {
		fill(u.y+v.ty+line.Rect.Y, line.Rect.H)
	}
}

func (e *Editor) paintFoldChevron(p *ui.Painter, h *leaf, ox, oy float32) {
	if h == nil || e.foldHover != h.lineStart || e.foldHeadingAt(e.pointerX, e.pointerY) != h {
		return
	}
	x, y, size, ok := e.foldChevronRect(h)
	if !ok {
		return
	}
	stroke := e.lk.muted.Alpha(0.82)
	path := ui.Path{}
	if e.isFoldedHeading(h) {
		path.MoveTo(ox+x+size*0.38, oy+y+size*0.22)
		path.LineTo(ox+x+size*0.68, oy+y+size*0.5)
		path.LineTo(ox+x+size*0.38, oy+y+size*0.78)
	} else {
		path.MoveTo(ox+x+size*0.22, oy+y+size*0.38)
		path.LineTo(ox+x+size*0.5, oy+y+size*0.68)
		path.LineTo(ox+x+size*0.78, oy+y+size*0.38)
	}
	p.StrokePath(&path, max(1.2, e.lk.size*0.075), stroke)
}

// paintInlineCodeBoxes paints one background per inline code or math range.
// Source delimiters keep their own muted text style over this shared shape,
// instead of drawing separate rounded span backgrounds.
func (e *Editor) paintInlineCodeBoxes(p *ui.Painter, u *leaf, v *leafView, x, y float32, ranges []Range) {
	lines := v.layout.Lines()
	for _, box := range v.disp.codeBoxes {
		for _, r := range v.layout.SelectionRects(box.range_) {
			padX := e.lk.size * 0.2
			baseline := r.Y + r.H/2
			bestDistance := float32(math.MaxFloat32)
			for _, line := range lines {
				lineCenter := line.Rect.Y + line.Rect.H/2
				distance := float32(math.Abs(float64(lineCenter - (r.Y + r.H/2))))
				if distance < bestDistance {
					bestDistance = distance
					baseline = line.Baseline
				}
			}
			h := box.size*1.32 + 2*box.padY
			pill := ui.Rect{
				X: x + r.X - padX,
				Y: y + baseline - box.size*0.98 - box.padY,
				W: r.W + 2*padX,
				H: h,
			}
			p.Fill(pill, e.lk.codeBg, max(3, e.lk.size/5))
			for _, selectionRange := range ranges {
				lo, hi := u.lineStart, u.end
				if selectionRange.Start >= hi || selectionRange.End <= lo {
					continue
				}
				da := v.disp.looseDisp(max(selectionRange.Start, lo) - lo)
				dz := v.disp.looseDisp(min(selectionRange.End, hi) - lo)
				selected := ui.TextInputRange{Start: max(da, box.range_.Start), End: min(dz, box.range_.End)}
				if selected.End > selected.Start {
					selection := e.lk.selection
					if !e.focused {
						selection = selection.Alpha(0.6)
					}
					for _, sr := range v.layout.SelectionRects(selected) {
						if math.Abs(float64(sr.Y+sr.H/2-(r.Y+r.H/2))) > float64(max(sr.H, r.H)/2) {
							continue
						}
						textLeft, textRight := x+sr.X, x+sr.X+sr.W
						if e.block != nil {
							p.Fill(ui.Rect{X: textLeft, Y: pill.Y, W: sr.W, H: pill.H}, selection, max(3, e.lk.size/5))
						}
						if selected.Start <= box.range_.Start && textLeft > pill.X {
							p.Fill(ui.Rect{X: pill.X, Y: pill.Y, W: textLeft - pill.X, H: pill.H}, selection, max(3, e.lk.size/5))
						}
						if selected.End >= box.range_.End && pill.X+pill.W > textRight {
							p.Fill(ui.Rect{X: textRight, Y: pill.Y, W: pill.X + pill.W - textRight, H: pill.H}, selection, max(3, e.lk.size/5))
						}
					}
				}
			}
		}
	}
}

// paintDiagramToggle draws the control at a Mermaid block's corner that
// shows its source, or brings its diagram back, a faint pill under it
// while the pointer is there.
func (e *Editor) paintDiagramToggle(p *ui.Painter, l *leaf, labelX, top float32) {
	action := locales.EditorSourceAction()
	if l.view != nil && l.view.diagram == nil {
		action = locales.EditorChartAction()
	}
	size := e.lk.size * 0.7
	sp := ui.Span{Text: action, Size: size, Color: e.lk.accent}
	w, h := p.MeasureText(0, sp)
	x := labelX - w - e.lk.size*0.8
	y := top + e.lk.size*0.35
	hot := e.pointerY >= l.y && e.pointerY <= l.y+e.lk.size*1.4 &&
		e.pointerX >= l.x && e.pointerX <= l.x+l.w
	if hot {
		p.Fill(ui.Rect{X: x - 4, Y: y - 2, W: w + 8, H: h + 4}, e.lk.accent.Alpha(0.12), 4)
	}
	p.RichText(x, y, 0, sp)
}

// diagramBox returns the box a diagram leaf draws in: the leaf's box
// without the code padding, at the origin ox, oy, and the diagram's
// natural width.
func (e *Editor) diagramBox(l *leaf, v *leafView, ox, oy float32) (ui.Rect, float32) {
	pad := e.lk.size * codePadY
	box := ui.Rect{X: ox + l.x, Y: oy + l.y + pad, W: l.w, H: max(l.h-2*pad, 1)}
	if v.diagramModel == nil {
		return box, 0
	}
	return box, v.diagramModel.Width()
}

// paintDiagram draws a diagram in its leaf, clipped to the column and
// scrolled sideways when it is wider than the column, with a scroll bar
// under it.
func (e *Editor) paintDiagram(p *ui.Painter, u *leaf, v *leafView, ox, oy float32) {
	box, contentW := e.diagramBox(u, v, ox, oy)
	off := float32(0)
	if contentW > box.W {
		off = min(max(v.diagX, 0), contentW-box.W)
	}
	p.Clip(box, 6, func() {
		v.diagram.Draw(p, ui.Rect{X: box.X - off, Y: box.Y, W: max(contentW, box.W), H: box.H}, p.Theme())
	})
	v.diagBarShown = v.diagIndicator.Visible(p.Now(), off, 0)
	if v.diagBarShown {
		p.After(v.diagIndicator.Remaining(p.Now()))
	}
	if contentW > box.W && v.diagBarShown {
		track, thumb := diagramBarRects(box, contentW, off)
		hot := track.Contains(e.pointerX, e.pointerY)
		e.paintDiagramBar(p, track, thumb, hot || e.diagDrag != nil && e.diagDrag.leaf == u)
	}
}

// diagramBarRects returns the track and the thumb of a diagram's scroll
// bar under box, for a diagram contentW wide scrolled by off.
func diagramBarRects(box ui.Rect, contentW, off float32) (track, thumb ui.Rect) {
	const barH = 5
	track = ui.Rect{X: box.X + 2, Y: box.Y + box.H + 2, W: max(box.W-4, 1), H: barH}
	frac := box.W / contentW
	tw := min(max(track.W*frac, 28), track.W)
	at := float32(0)
	if maxOff := contentW - box.W; maxOff > 0 {
		at = min(max(off/maxOff, 0), 1)
	}
	thumb = ui.Rect{X: track.X + (track.W-tw)*at, Y: track.Y, W: tw, H: track.H}
	return track, thumb
}

// paintDiagramBar draws a diagram's scroll bar: a faint track, and a thumb
// showing how much of the diagram shows.
func (e *Editor) paintDiagramBar(p *ui.Painter, track, thumb ui.Rect, hot bool) {
	col := p.Theme().Scrollbar
	if hot {
		col = e.lk.accent.Alpha(0.7)
	}
	p.Fill(track, e.lk.border.Alpha(0.16), 2.5)
	p.Fill(thumb, col, 2.5)
}

// fillRange fills the part of the source range [a, z) that shows in a
// unit.
func (e *Editor) fillRange(p *ui.Painter, u *leaf, v *leafView, a, z int, lx, ly float32, col ui.Color) {
	e.fillRangePulsed(p, u, v, a, z, lx, ly, col, 0)
}

func (e *Editor) fillRangePulsed(p *ui.Painter, u *leaf, v *leafView, a, z int, lx, ly float32, col ui.Color, pulse float32) {
	lo, hi := u.lineStart, u.end
	// A source-open fenced-code view can draw the closing fence after the
	// parser's code leaf ends. Those source-backed pieces are still part of
	// this view and must receive the same selection paint as the code body.
	for _, pc := range v.disp.pieces {
		hi = max(hi, u.lineStart+pc.s1)
	}
	if z < lo || a > hi || v.empty {
		return
	}
	da := v.disp.looseDisp(max(a, lo) - u.lineStart)
	dz := v.disp.looseDisp(min(z, hi) - u.lineStart)
	if dz <= da {
		return
	}
	for _, r := range v.layout.SelectionRects(ui.TextInputRange{Start: da, End: dz}) {
		r.X += lx
		r.Y += ly
		padX, padY := 2*pulse, pulse
		r.X -= padX
		r.Y -= padY
		r.W += 2 * padX
		r.H += 2 * padY
		p.Fill(r, col, 2+2*pulse)
	}
}

// looseDisp returns the display offset of any source offset (relative): of
// the piece holding it, or the edge of the next piece.
func (d *display) looseDisp(rel int) int {
	for i := range d.pieces {
		pc := &d.pieces[i]
		if rel > pc.s1 {
			continue
		}
		if rel <= pc.s0 {
			return pc.d0
		}
		if pc.kind == pText {
			return pc.d0 + ui.UTF16Len(pc.text[:min(rel-pc.s0, len(pc.text))])
		}
		return pc.d1
	}
	if len(d.pieces) == 0 {
		return 0
	}
	return d.pieces[len(d.pieces)-1].d1
}

// paintObjects draws the images and formulas inside a layout's lines.
func (e *Editor) paintObjects(p *ui.Painter, u *leaf, v *leafView, lx, ly float32) {
	if !v.disp.hasObjects {
		return
	}
	for _, o := range v.layout.Objects() {
		for i := range v.disp.pieces {
			pc := &v.disp.pieces[i]
			if pc.obj != nil && pc.d0 == o.Index {
				if pc.obj.spacer {
					break
				}
				r := o.Rect
				r.X += lx
				r.Y += ly
				e.paintObject(p, pc.obj, r)
				if pc.obj.imagePath != "" {
					e.imageHits = append(e.imageHits, imageHit{obj: pc.obj, box: ui.Rect{X: r.X - e.elemRect.X, Y: r.Y - e.elemRect.Y, W: r.W, H: r.H}, leaf: u})
				}
				break
			}
		}
	}
}

func (e *Editor) paintImageTools(p *ui.Painter, hit imageHit, ox, oy float32) {
	box := hit.box
	box.X += e.elemRect.X
	box.Y += e.elemRect.Y
	preview, source, align, grip := imageToolRects(box, e.lk.size)
	items := []struct {
		r     ui.Rect
		label string
	}{{preview, "↗"}}
	if hit.obj != nil && hit.obj.block {
		items = append(items, struct {
			r     ui.Rect
			label string
		}{align, map[bool]string{true: "↔", false: "↤"}[hit.obj.alignCenter]})
	}
	items = append(items, struct {
		r     ui.Rect
		label string
	}{source, "</>"})
	for _, item := range items {
		p.Fill(item.r, e.lk.panel, 5)
		p.Stroke(item.r, e.lk.border, 5, 1)
		sp := ui.Span{Text: item.label, Size: e.lk.size * 0.7, Color: e.lk.text}
		w, h := p.MeasureText(item.r.W-4, sp)
		p.RichText(item.r.X+(item.r.W-w)/2, item.r.Y+(item.r.H-h)/2, item.r.W-4, sp)
	}
	// A quiet L mark at the lower-right corner is the image resize affordance.
	col := e.lk.text.Alpha(0.72)
	p.Fill(ui.Rect{X: grip.X + grip.W - 3, Y: grip.Y + grip.H*0.42, W: 2, H: grip.H * 0.48}, col, 1)
	p.Fill(ui.Rect{X: grip.X + grip.W*0.42, Y: grip.Y + grip.H - 3, W: grip.W * 0.48, H: 2}, col, 1)
}

// paintObject draws an image or a formula in r, or a placeholder for an
// image not loaded.
func (e *Editor) paintObject(p *ui.Painter, o *object, r ui.Rect) {
	o.refreshAssetBitmap()
	if o.embedBlocked != "" {
		p.Fill(r, e.lk.panel, 7)
		p.Stroke(r, e.lk.border, 7, 1)
		pad := e.lk.size * 0.65
		sp := ui.Span{Text: o.embedBlocked, Size: e.lk.size * 0.82, Color: e.lk.muted}
		_, h := p.MeasureText(max(r.W-2*pad, 1), sp)
		p.RichText(r.X+pad, r.Y+(r.H-h)/2, max(r.W-2*pad, 1), sp)
		return
	}
	if o.embed != nil {
		pad := e.lk.size * 0.55
		p.Fill(r, e.lk.panel, 7)
		p.Stroke(r, e.lk.border.Alpha(0.7), 7, 1)
		inner := ui.Rect{X: r.X + pad, Y: r.Y + pad, W: max(r.W-2*pad, 1), H: max(r.H-2*pad, 1)}
		p.Clip(inner, 4, func() { o.embed.paint(p, inner) })
		return
	}
	if o.footnote {
		sp := ui.Span{Text: o.alt, Size: e.lk.size * 0.78, Color: e.lk.accent}
		ascent := ui.Font{Family: e.lk.font, Size: sp.Size}.Metrics().Ascent
		p.RichText(r.X, r.Y-ascent-e.lk.size*0.4, r.W, sp)
		return
	}
	if o.math != nil {
		o.math.Draw(p, r)
		return
	}
	if o.bmp != nil {
		p.Image(o.bmp, r, ui.Contain)
		return
	}
	p.Fill(r, e.lk.panel, 6)
	if o.imagePending {
		label := o.alt
		if label == "" {
			label = locales.EditorImageLoading()
		}
		sp := ui.Span{Text: label, Size: e.lk.size * 0.8, Color: e.lk.muted}
		pad := e.lk.size * 0.8
		textW := max(r.W-2*pad, 0)
		_, h := p.MeasureText(textW, sp)
		p.Clip(r, 6, func() {
			p.RichText(r.X+pad, r.Y+(r.H-h)/2, textW, sp)
		})
		return
	}
	source := o.imageDest
	if source == "" {
		source = o.imagePath
	}
	sp := ui.Span{Text: missingImageMessage(source), Size: e.lk.size * 0.8, Color: e.lk.muted}
	pad := e.lk.size * 0.8
	textW := max(r.W-2*pad, 0)
	_, h := p.MeasureText(textW, sp)
	p.Clip(r, 6, func() {
		p.RichText(r.X+pad, r.Y+(r.H-h)/2, textW, sp)
	})
}

// paintSourceChip draws a block's source control: the little `</>` button
// that shows the block as the Markdown it is written as.
func (e *Editor) paintSourceChip(p *ui.Painter, l *leaf, ox, oy float32) {
	r := e.sourceChip(l)
	r.X += ox
	r.Y += oy
	p.Fill(r, e.lk.panel, 5)
	p.Stroke(r, e.lk.border.Alpha(1), 5, 1)
	sp := ui.Span{Text: "</>", Size: e.lk.size * 0.72, Color: e.lk.muted}
	w, h := p.MeasureText(r.W-4, sp)
	p.RichText(r.X+(r.W-w)/2, r.Y+(r.H-h)/2, r.W-4, sp)
}

// paintMarkers draws the markers of the items a leaf opens: bullets,
// numbers, check boxes, and a footnote's label.
func (e *Editor) paintMarkers(p *ui.Painter, l *leaf, ox, oy float32) {
	if len(l.firstOf) == 0 {
		return
	}
	baseline := l.y + e.lk.size*1.1
	if v := l.view; v != nil && v.layout != nil {
		if lines := v.layout.Lines(); len(lines) > 0 {
			baseline = l.y + v.ty + lines[0].Baseline
		}
	}
	for _, k := range l.firstOf {
		c := l.conts[k]
		if (c.kind == cItem || c.kind == cFootnote) && e.containerPrefixOpen(l) {
			continue
		}
		x0 := e.colX + e.indentOf(l.conts[:k])
		w := e.indentOf(l.conts[k : k+1])
		switch c.kind {
		case cItem:
			e.paintItemMarker(p, c, ox+x0, oy+baseline, w)
		case cFootnote:
			sp := ui.Span{Text: c.label + ":", Size: e.lk.size, Color: e.lk.accent}
			fm := ui.Font{Family: e.lk.font, Size: sp.Size}.Metrics()
			p.RichText(ox+x0, oy+baseline-fm.Ascent, w, sp)
		}
	}
}

// paintItemMarker draws an item's marker in the column from x, w wide, on
// the baseline.
func (e *Editor) paintItemMarker(p *ui.Painter, c cont, x, baseline, w float32) {
	s := e.lk.size
	if c.task != syntax.IncrementalNoTask {
		box := s * 0.95
		r := ui.Rect{X: x + w - box - s*0.45, Y: baseline - s*0.82, W: box, H: box}
		if c.task == syntax.IncrementalChecked {
			p.Fill(r, e.lk.accent, 3.5)
			path := ui.Path{}
			path.MoveTo(r.X+r.W*0.24, r.Y+r.H*0.52)
			path.LineTo(r.X+r.W*0.43, r.Y+r.H*0.7)
			path.LineTo(r.X+r.W*0.77, r.Y+r.H*0.32)
			p.StrokePath(&path, 1.8, theme.From(e.lk.accent, e.lk.dark).OnAccent)
		} else {
			p.Stroke(r, e.lk.muted.Alpha(0.8), 3.5, 1.3)
		}
		return
	}
	text := "•"
	size := s
	if c.ordered {
		text = strconv.Itoa(c.num) + string(c.marker)
	} else {
		switch c.depth % 3 {
		case 1:
			text = "◦"
		case 2:
			text = "▪"
			size = s * 0.8
		}
	}
	sp := ui.Span{Text: text, Size: size, Color: e.lk.muted}
	if c.ordered {
		sp.Features = "tnum"
	}
	fm := ui.Font{Family: e.lk.font, Size: size}.Metrics()
	tw, _ := p.MeasureText(0, sp)
	p.RichText(x+w-tw-s*0.45, baseline-fm.Ascent, 0, sp)
}

// paintTable draws a table's grid and its header row's fill.
func (e *Editor) paintTable(p *ui.Painter, l *leaf, ox, oy float32, t *ui.Theme) {
	g := l.geom
	if g == nil {
		return
	}
	x, y := ox+tableContentX(l), oy+l.y
	h := g.rowY[len(g.rowY)-1]
	box := ui.Rect{X: x, Y: y, W: g.w, H: h}
	const radius = 4
	line := uiMarkdownColor(theme.MarkdownColors(e.lk.dark).TableRule)
	if s := e.tableStructureSelection(l); s != nil {
		selection := e.lk.selection.Alpha(0.4)
		if !e.focused {
			selection = selection.Alpha(0.6)
		}
		selected := box
		switch s.axis {
		case tableCells:
			r, ok := e.tableSelectionCellRegion(s)
			if ok {
				top, left, bottom, right := r.bounds()
				top = max(0, min(top, len(g.rowY)-2))
				bottom = max(top, min(bottom, len(g.rowY)-2))
				left = max(0, min(left, len(g.colX)-2))
				right = max(left, min(right, len(g.colX)-2))
				selected = ui.Rect{X: x + g.colX[left], Y: y + g.rowY[top], W: g.colX[right+1] - g.colX[left], H: g.rowY[bottom+1] - g.rowY[top]}
			}
		case tableRows:
			first, last := s.first, s.last
			selected.Y = y + g.rowY[first]
			selected.H = g.rowY[last+1] - g.rowY[first]
		case tableColumns:
			first, last := s.first, s.last
			selected.X = x + g.colX[first]
			selected.W = g.colX[last+1] - g.colX[first]
		}
		if s.axis == tableCells {
			clip := ui.Rect{X: ox + l.x, Y: y, W: tableViewportWidth(l), H: h}
			p.Clip(clip, 0, func() {
				p.Fill(selected, selection, radius)
				p.Stroke(selected, e.lk.accent.Alpha(0.6), radius, 1.5)
			})
		} else {
			p.Fill(selected, selection, radius)
			p.Stroke(selected, e.lk.accent.Alpha(0.6), radius, 1.5)
		}
	}
	for i, ry := range g.rowY {
		if i == 0 || i == len(g.rowY)-1 {
			continue
		}
		p.Fill(ui.Rect{X: x, Y: y + ry - 0.5, W: g.w, H: 1}, line, 0)
	}
	for i, cx := range g.colX {
		if i == 0 || i == len(g.colX)-1 {
			continue
		}
		p.Fill(ui.Rect{X: x + cx - 0.5, Y: y, W: 1, H: h}, line, 0)
	}
	p.Stroke(box, line, radius, 1)
}

// paintCaret draws the caret, blinking, while the editor has the focus.
func (e *Editor) paintCaret(p *ui.Painter, ox, oy float32) {
	if e.compose == "" && e.tableBridge == nil {
		if rowCaret, ok := e.tableSelectionCaretRect(); ok {
			// Structural row selection is its own focus state. Keep the active
			// boundary visible even if the text editor has temporarily lost focus.
			e.paintTableBoundaryCaret(p, ox, oy, rowCaret)
			return
		}
	}
	if !e.focused {
		return
	}
	var nextRedraw time.Duration
	defer func() {
		if nextRedraw > 0 {
			p.After(nextRedraw)
		}
	}()
	scheduleRedraw := func(after time.Duration) {
		if after > 0 && (nextRedraw == 0 || after < nextRedraw) {
			nextRedraw = after
		}
	}
	if e.compose == "" {
		if _, _, _, boundary := e.tableBoundaryAtOffset(e.caret); boundary {
			if r, ok := e.caretRect(e.caret); ok {
				e.paintTableBoundaryCaret(p, ox, oy, r)
				return
			}
		} else if t, _ := e.tableAtEdge(e.caret); t != nil {
			if r, ok := e.caretRect(e.caret); ok {
				e.paintTableBoundaryCaret(p, ox, oy, r)
				return
			}
		}
	}
	phase := p.Now().Sub(e.blinkStart)
	if phase < 30*time.Second {
		untilTransition := blink - phase%blink
		if e.opts.CursorFade {
			if _, fading := caretFadeOpacity(phase); fading {
				// Painter callbacks are the editor's animation clock. Keep the
				// caret repainting during the short opacity ramp, not just at
				// the on/off boundary.
				scheduleRedraw(16 * time.Millisecond)
			} else {
				scheduleRedraw(untilTransition)
			}
		} else {
			scheduleRedraw(untilTransition)
		}
	}
	caretAlpha := float32(1)
	if e.opts.CursorFade && e.compose == "" {
		caretAlpha, _ = caretFadeOpacity(phase)
	} else if e.compose == "" && phase%(2*blink) >= blink {
		return
	}
	if e.anchor != e.caret && e.compose == "" && !e.blockCaret() {
		return
	}
	r, ok := e.composeCaretRect()
	if !ok {
		r, ok = e.caretRect(e.caret)
	}
	if !ok {
		return
	}
	if e.compose == "" && smoothCursorNeedsFrames(e.opts.SmoothCursor, e.smoothCursorActive) {
		from, fromOK := e.smoothCursorFromRect, e.smoothCursorFromSet
		if !fromOK {
			from, fromOK = e.caretRect(e.smoothCursorFrom)
		}
		if fromOK {
			target := r
			if visual, active := e.smoothCursorRects(p.Now(), from, r); active {
				r = visual
				e.smoothCursorFromRect = from
				e.smoothCursorToRect = target
				e.smoothCursorRectsValid = true
				p.AnimationFrameRate(smoothCursorFrameRate)
			} else {
				e.smoothCursorRectsValid = false
			}
		} else {
			e.smoothCursorActive = false
			e.smoothCursorRectsValid = false
		}
	}
	if e.blockCaret() && e.compose == "" {
		r = e.blockCaretRect(r)
		box := ui.Rect{X: ox + r.X, Y: oy + r.Y, W: r.W, H: r.H}
		p.Fill(box, e.lk.caret.Alpha(caretAlpha), 0)
		// Repaint the shaped text through the caret box so the glyph itself
		// takes a contrasting foreground on the opaque caret background.
		if us := e.units(); len(us) > 0 {
			if i := e.unitIndex(us, e.caret); i >= 0 {
				u := us[i]
				v := e.unitView(u, e.caret)
				if v.layout != nil {
					if d, ok := v.disp.srcToDisp(e.caret - u.lineStart); ok {
						if sp, ok := blockCaretGlyph(v.disp, d, blockCaretTextColor(e.lk.caret).Alpha(caretAlpha)); ok {
							glyph := ui.ShapeRichText([]ui.Span{sp}, v.font, 0)
							lines, glyphLines := v.layout.Lines(), glyph.Lines()
							if len(lines) > 0 && len(glyphLines) > 0 {
								ln := lines[lineOf(lines, d, e.upstream)]
								y := oy + u.y + v.ty + ln.Baseline - glyphLines[0].Baseline
								p.Clip(box, 0, func() {
									p.TextLayout(glyph, box.X, y, blockCaretTextColor(e.lk.caret).Alpha(caretAlpha))
								})
							}
						}
					}
				}
			}
		}
		return
	}
	h := r.H
	p.Fill(ui.Rect{X: ox + r.X - 0.75, Y: oy + r.Y, W: 1.75, H: h}, e.lk.caret.Alpha(caretAlpha), 0.5)
}

// caretFadeOpacity returns the caret opacity during the short fade windows at
// the beginning and middle of the blink cycle. Away from those boundaries the
// caret is fully visible or hidden. The matching easing on both edges avoids a
// mechanical hard cut while preserving the existing blink period.
func caretFadeOpacity(phase time.Duration) (float32, bool) {
	const fade = 120 * time.Millisecond
	// A caret relocation resets blinkStart. Keep that freshly moved caret fully
	// visible instead of restarting the cycle at zero opacity on every keypress.
	if phase < fade {
		return 1, false
	}
	cycle := phase % (2 * blink)
	if cycle < fade {
		t := float32(cycle) / float32(fade)
		return t * t * (3 - 2*t), true
	}
	if cycle >= blink && cycle < blink+fade {
		t := float32(cycle-blink) / float32(fade)
		t = 1 - t*t*(3-2*t)
		return t, true
	}
	if cycle >= blink+fade {
		return 0, false
	}
	return 1, false
}

func (e *Editor) paintTableBoundaryCaret(p *ui.Painter, ox, oy float32, r ui.Rect) {
	// A table boundary has no character cell to invert. Use the same thin
	// accent stroke for every side and row, independent of the modal mode.
	p.Fill(ui.Rect{X: ox + r.X - 0.75, Y: oy + r.Y, W: 2.25, H: r.H}, e.lk.caret, 0.5)
}

// composeCaretRect returns the caret's box inside the composition of an
// input method.
func (e *Editor) composeCaretRect() (ui.Rect, bool) {
	if e.compose == "" {
		return ui.Rect{}, false
	}
	us := e.units()
	i := e.unitIndex(us, e.caret)
	if i < 0 {
		return ui.Rect{}, false
	}
	u := us[i]
	v := u.view
	if v == nil || v.layout == nil {
		return ui.Rect{}, false
	}
	for _, pc := range v.disp.pieces {
		if pc.kind == pDeco && pc.text == e.compose && pc.s0+u.lineStart == e.caret {
			r := v.layout.Caret(pc.d0 + min(e.composeSel.End, pc.d1-pc.d0))
			r.X += u.x + v.tx
			r.Y += u.y + v.ty
			return r, true
		}
	}
	return ui.Rect{}, false
}
