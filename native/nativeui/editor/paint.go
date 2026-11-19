package editor

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
	helix "github.com/wheregrassdreams/muse-editor/text/helix"

	"muse/locales"
	media "muse/nativeui/editor/blocks/media"
	paragraphui "muse/nativeui/editor/blocks/paragraph"
	"muse/nativeui/editor/feedback"
	render "muse/nativeui/editor/render"
	"muse/nativeui/editor/textlayout"
	"muse/theme"
)

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
	scratch := &e.renderScratch
	scratch.reset()
	e.imageHits = e.imageHits[:0]
	e.elemRect = r
	// The scroll handle can still report its provisional first-frame width
	// while MyGo has resolved the actual draw bounds. Recompute the centered
	// column and its layout before painting, so the first visible frame has the
	// same padding as subsequent frames without waiting for user interaction.
	e.setElementWidth(r.W)
	// An app edit or the width correction above may have landed after View
	// built this element. Pair the current source/parser and geometry before
	// reading leaves retained by this draw.
	e.layoutIfNeeded()
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
		scratch.addOverlay(render.Overlay{
			Kind: render.FillOverlay, Bounds: ui.Rect{X: ox + b.x + 1, Y: oy + b.y0 - 2, W: 2, H: b.y1 - b.y0 + 4},
			Color: e.lk.quoteBar, Radius: 1,
		}, false)
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
			scratch.clip = clip
			drawBlock()
			scratch.clip = ui.Rect{}
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
		if !hit.box.Contains(e.pointerX, e.pointerY) && (e.imageDrag == nil || e.imageDrag.hit.obj.imageSource.From != hit.obj.imageSource.From || e.imageDrag.hit.obj.imageSource.To != hit.obj.imageSource.To) {
			continue
		}
		e.paintImageTools(p, hit, ox, oy)
	}
	if e.Source() == "" && e.opts.Placeholder != "" && e.compositionText() == "" {
		// The placeholder of an empty note stands on the first line of the
		// document, where the note begins — not wherever the caret's line
		// is: it marks the empty note, and it does not move.
		x, w := e.colX, e.colW
		if len(e.leaves) > 0 {
			x, w = e.leaves[0].x, e.leaves[0].w
		}
		sp := ui.Span{Text: e.opts.Placeholder, Color: e.lk.muted.Alpha(0.6), Size: e.lk.size}
		e.addRenderOverlay(render.Overlay{Kind: render.TextOverlay, Bounds: ui.Rect{X: ox + x, Y: oy + e.padTop + e.lk.size*e.lk.lh/2 - e.lk.size*.7}, MaxW: w, Text: sp}, true)
	}
	if !e.smoothCursorOverlayEnabled() {
		e.paintCaretLayer(p, ox, oy)
	}
	snapshot := scratch.snapshot(e, r, e.renderStyle())
	frame := e.renderer.Build(snapshot, scratch.resources.ports())
	render.Paint(p, frame, scratch.resources.ports())
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
	start, end := render.VisibleRange(len(leaves), func(i int) render.VerticalBounds {
		return render.VerticalBounds{Y: yAt(i), H: leaves[i].h}
	}, top, bottom)
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
	state := e.VimViewState()
	return e.opts.SmoothCursor && (state.Keymap == "vim" || state.Keymap == "helix") && state.Mode == helix.ModeNormal
}

// paintCaretLayer is a separate retained scene element when smooth cursor is
// enabled, so Painter.After redraws only this small overlay instead of every
// document leaf.
func (e *Editor) paintCaretLayer(p *ui.Painter, ox, oy float32) {
	if l := e.horizontalOwner(e.selectionFocus()); l != nil && blockContentWidth(l) > blockViewport(l).W {
		clip := blockViewport(l)
		clip.X, clip.Y, clip.W = clip.X+ox-2, clip.Y+oy, clip.W+4
		e.paintCaret(p, ox, oy, clip)
	} else {
		e.paintCaret(p, ox, oy, ui.Rect{})
	}
}

// paintUnit draws a leaf or a cell: its panel, what is found and selected
// in it, its text and its objects.
func (e *Editor) paintUnit(p *ui.Painter, u *leaf, ox, oy float32) {
	a, z := e.textSelectionForUnit(u)
	yankOpacity, yankActive := e.vimYankFlashOpacity(p.Now())
	v := u.view
	if v == nil {
		e.paintBlockRowBackground(p, u, nil, ox, oy)
		if yankActive && e.yankFlashTouches(u) {
			box := ui.Rect{X: ox + u.x, Y: oy + u.y, W: u.w, H: u.h}
			e.addRenderOverlay(render.Overlay{Kind: render.FillOverlay, Bounds: box, Color: e.lk.accent.Alpha(.32 * yankOpacity), Radius: 3}, false)
		}
		// A blank line has no text and no view: nothing to draw — but a
		// selection across it covers the line, and shows as covering it.
		if e.selectionCoversBlankLine(u, a, z) {
			e.addRenderOverlay(render.Overlay{Kind: render.FillOverlay, Bounds: ui.Rect{X: ox + u.x, Y: oy + u.y, W: u.w, H: u.h}, Color: e.lk.selection, Radius: 2}, false)
		}
		return
	}
	x, y := ox+u.x, oy+u.y
	contentX := x - blockScrollOffset(u)
	box := ui.Rect{X: x, Y: y, W: u.w, H: u.h}
	if v.panel {
		e.addRenderOverlay(render.Overlay{Kind: render.FillOverlay, Bounds: box, Color: e.lk.panel, Radius: 6}, false)
	}
	e.paintBlockRowBackground(p, u, v, ox, oy)
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
		e.addRenderOverlay(render.Overlay{Kind: render.FillOverlay, Bounds: ui.Rect{X: x, Y: y + u.h/2 - .75, W: u.w, H: 1.5}, Color: e.lk.border}, false)
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
		if v.diagramModel == nil {
			e.paintLayout(p, u, v, lx, ly)
		}
		e.paintObjects(p, u, v, lx, ly)
	} else if a != z && a <= leafLo(e, u) && z >= u.end && !(e.tableSelection != nil && u.table != nil && e.tableSelection.lineStart == u.table.lineStart) {
		e.addRenderOverlay(render.Overlay{Kind: render.FillOverlay, Bounds: box, Color: e.lk.selection, Radius: 4}, false)
	}
	if v.block != nil {
		r := ui.Rect{X: contentX + v.bx, Y: y + v.by, W: v.block.w, H: v.block.h}
		e.paintObject(p, v.block, r)
		if v.block.imagePath != "" {
			e.imageHits = append(e.imageHits, imageHit{obj: v.block, box: ui.Rect{X: r.X - e.elemRect.X, Y: r.Y - e.elemRect.Y, W: r.W, H: r.H}, leaf: u})
		}
	}
	if v.diagramModel != nil {
		e.paintDiagram(p, u, v, ox, oy)
	}
	if yankActive {
		e.paintVimYankFlash(p, u, v, contentX+v.tx, y+v.ty, box, yankOpacity)
	}
	if state, ok := e.vimImageRowState(); ok && state.LineStart == u.lineStart && a < z {
		lineStart, lineEnd := e.sourceLineRange(u.lineStart)
		if a <= lineEnd && z > lineStart {
			if image, _, ok := e.vimImageGeometry(u); ok {
				e.addRenderOverlay(render.Overlay{Kind: render.FillOverlay, Bounds: ui.Rect{X: ox + image.X, Y: oy + image.Y, W: image.W, H: image.H}, Color: e.lk.selection.Alpha(.28), Radius: 4}, false)
			}
		}
	}
	if v.label != "" && !v.reveal && v.panel {
		size := e.lk.size
		sp := ui.Span{Text: v.label, Size: size * 0.7, Color: e.lk.muted.Alpha(0.75)}
		w, _ := p.MeasureText(0, sp)
		lx := x + u.w - w - size*0.6
		e.addRenderOverlay(render.Overlay{Kind: render.TextOverlay, Bounds: ui.Rect{X: lx, Y: y + size*.35}, Text: sp}, true)
		if isMermaid(u) {
			e.paintDiagramToggle(p, u, lx, y)
		}
	}
}

// paintLayout adapts the cached, already-shaped visible layout into the
// renderer's detached frame. The frame is synchronous and short-lived; the
// renderer never retains its layout or any leaf/source reference.
func (e *Editor) paintLayout(p *ui.Painter, l *leaf, v *leafView, x, y float32) {
	if v == nil || v.layout == nil {
		return
	}
	w, h := v.layout.Size()
	block := render.Block{
		Kind: render.TextBlock, Bounds: ui.Rect{X: x, Y: y, W: max(w, l.w), H: max(h, l.h)},
		Layout: v.layout, TextX: x, TextY: y,
	}
	e.renderScratch.addBlock(block)
}

func (e *Editor) yankFlashTouches(u *leaf) bool {
	lo, hi := leafLo(e, u), u.end
	for _, r := range e.feedbackState.YankRanges() {
		if r.Start < hi && r.End > lo {
			return true
		}
	}
	return false
}

func (e *Editor) paintVimYankFlash(p *ui.Painter, u *leaf, v *leafView, lx, ly float32, box ui.Rect, opacity float32) {
	col := e.lk.accent.Alpha(0.32 * opacity)
	for _, r := range e.feedbackState.YankRanges() {
		if r.Start >= u.end || r.End <= leafLo(e, u) {
			continue
		}
		if v.layout != nil {
			e.fillRangeInFront(p, u, v, r.Start, r.End, lx, ly, col)
		} else {
			e.addRenderOverlay(render.Overlay{Kind: render.FillOverlay, Bounds: box, Color: col, Radius: 3}, true)
		}
	}
}

func (e *Editor) selectionCoversBlankLine(u *leaf, a, z int) bool {
	if u == nil || a == z {
		return false
	}
	start, end := e.sourceLineRange(u.start)
	if end < len(e.Source()) {
		end++ // The linewise range owns the line break ending this blank line.
	}
	return start < end && a <= start && z >= end
}

func (e *Editor) paintBlockRowBackground(p *ui.Painter, u *leaf, v *leafView, ox, oy float32) {
	overlays := e.blockRowOverlays(u, v, ox, oy)
	if len(overlays) != 0 {
		for _, overlay := range overlays {
			e.addRenderOverlay(overlay, false)
		}
	}
}

func (e *Editor) blockRowOverlays(u *leaf, v *leafView, ox, oy float32) []render.Overlay {
	b := e.block
	if b == nil || u == nil || u.table != nil || u.kind == kTable {
		return nil
	}
	col := e.lk.selection
	if !e.focused {
		col = col.Alpha(0.6)
	}
	overlays := make([]render.Overlay, 0, 2)
	fill := func(y, h float32) {
		if y+h/2+0.1 < b.top || y+h/2-0.1 > b.bottom {
			return
		}
		overlays = append(overlays, render.Overlay{
			Kind: render.FillOverlay, Bounds: ui.Rect{X: ox + b.left, Y: oy + y, W: max(b.right-b.left, 1), H: max(h, 1)},
			Color: col, Radius: 2,
		})
	}
	if v == nil || v.layout == nil {
		fill(u.y, u.h)
		return overlays
	}
	for _, line := range v.layout.Lines() {
		fill(u.y+v.ty+line.Rect.Y, line.Rect.H)
	}
	return overlays
}

func (e *Editor) paintFoldChevron(p *ui.Painter, h *leaf, ox, oy float32) {
	if h == nil || e.foldHover != h.lineStart {
		return
	}
	stroke := e.lk.muted.Alpha(0.82)
	paragraphui.PaintFoldChevron(p, e.foldFrame(), h.lineStart, e.pointerX, e.pointerY, ox, oy, stroke, max(1.2, e.lk.size*0.075))
}

// paintInlineCodeBoxes paints one background per inline code or math range.
// Source delimiters keep their own muted text style over this shared shape,
// instead of drawing separate rounded span backgrounds.
func (e *Editor) paintInlineCodeBoxes(p *ui.Painter, u *leaf, v *leafView, x, y float32, ranges []Range) {
	lines := v.layout.Lines()
	overlays := make([]render.Overlay, 0, len(v.disp.codeBoxes))
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
			radius := max(3, e.lk.size/5)
			overlays = append(overlays, render.Overlay{Kind: render.FillOverlay, Bounds: pill, Color: e.lk.codeBg, Radius: radius})
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
							overlays = append(overlays, render.Overlay{Kind: render.FillOverlay, Bounds: ui.Rect{X: textLeft, Y: pill.Y, W: sr.W, H: pill.H}, Color: selection, Radius: radius})
						}
						if selected.Start <= box.range_.Start && textLeft > pill.X {
							overlays = append(overlays, render.Overlay{Kind: render.FillOverlay, Bounds: ui.Rect{X: pill.X, Y: pill.Y, W: textLeft - pill.X, H: pill.H}, Color: selection, Radius: radius})
						}
						if selected.End >= box.range_.End && pill.X+pill.W > textRight {
							overlays = append(overlays, render.Overlay{Kind: render.FillOverlay, Bounds: ui.Rect{X: textRight, Y: pill.Y, W: pill.X + pill.W - textRight, H: pill.H}, Color: selection, Radius: radius})
						}
					}
				}
			}
		}
	}
	if len(overlays) > 0 {
		for _, overlay := range overlays {
			e.addRenderOverlay(overlay, false)
		}
	}
}

// paintDiagramToggle draws the control at a Mermaid block's corner that
// shows its source, or brings its diagram back, a faint pill under it
// while the pointer is there.
func (e *Editor) paintDiagramToggle(p *ui.Painter, l *leaf, labelX, top float32) {
	action := locales.EditorSourceAction()
	if l.view != nil && l.view.diagramModel == nil {
		action = locales.EditorChartAction()
	}
	size := e.lk.size * 0.7
	sp := ui.Span{Text: action, Size: size, Color: e.lk.accent}
	w, h := p.MeasureText(0, sp)
	x := labelX - w - e.lk.size*0.8
	y := top + e.lk.size*0.35
	hot := e.pointerY >= l.y && e.pointerY <= l.y+e.lk.size*1.4 &&
		e.pointerX >= l.x && e.pointerX <= l.x+l.w
	overlays := make([]render.Overlay, 0, 2)
	if hot {
		overlays = append(overlays, render.Overlay{Kind: render.FillOverlay, Bounds: ui.Rect{X: x - 4, Y: y - 2, W: w + 8, H: h + 4}, Color: e.lk.accent.Alpha(0.12), Radius: 4})
	}
	overlays = append(overlays, render.Overlay{Kind: render.TextOverlay, Bounds: ui.Rect{X: x, Y: y}, Text: sp})
	for _, overlay := range overlays {
		e.addRenderOverlay(overlay, true)
	}
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
	block := render.Block{Kind: render.DiagramBlock, Bounds: ui.Rect{X: box.X - off, Y: box.Y, W: max(contentW, box.W), H: box.H}, Clip: box, Resource: e.renderScratch.resources.addDiagram(e.diagramVisual(v.diagramModel))}
	e.renderScratch.addBlock(block)
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
	e.addRenderOverlay(render.Overlay{Kind: render.FillOverlay, Bounds: track, Color: e.lk.border.Alpha(0.16), Radius: 2.5}, true)
	e.addRenderOverlay(render.Overlay{Kind: render.FillOverlay, Bounds: thumb, Color: col, Radius: 2.5}, true)
}

// fillRange fills the part of the source range [a, z) that shows in a
// unit.
func (e *Editor) fillRange(p *ui.Painter, u *leaf, v *leafView, a, z int, lx, ly float32, col ui.Color) {
	e.fillRangePulsed(p, u, v, a, z, lx, ly, col, 0)
}

func (e *Editor) fillRangePulsed(p *ui.Painter, u *leaf, v *leafView, a, z int, lx, ly float32, col ui.Color, pulse float32) {
	overlays := e.rangeOverlays(u, v, a, z, lx, ly, col, pulse)
	for _, overlay := range overlays {
		e.addRenderOverlay(overlay, false)
	}
}

func (e *Editor) fillRangeInFront(p *ui.Painter, u *leaf, v *leafView, a, z int, lx, ly float32, col ui.Color) {
	overlays := e.rangeOverlays(u, v, a, z, lx, ly, col, 0)
	for _, overlay := range overlays {
		e.addRenderOverlay(overlay, true)
	}
}

func (e *Editor) rangeOverlays(u *leaf, v *leafView, a, z int, lx, ly float32, col ui.Color, pulse float32) []render.Overlay {
	lo, hi := u.lineStart, u.end
	// A source-open fenced-code view can draw the closing fence after the
	// parser's code leaf ends. Those source-backed pieces are still part of
	// this view and must receive the same selection paint as the code body.
	for _, pc := range v.disp.pieces {
		hi = max(hi, u.lineStart+pc.s1)
	}
	if z < lo || a > hi || v.empty {
		return nil
	}
	da := v.disp.looseDisp(max(a, lo) - u.lineStart)
	dz := v.disp.looseDisp(min(z, hi) - u.lineStart)
	if dz <= da {
		return nil
	}
	rects := v.layout.SelectionRects(ui.TextInputRange{Start: da, End: dz})
	overlays := make([]render.Overlay, 0, len(rects))
	for _, r := range rects {
		r.X += lx
		r.Y += ly
		padX, padY := 2*pulse, pulse
		r.X -= padX
		r.Y -= padY
		r.W += 2 * padX
		r.H += 2 * padY
		overlays = append(overlays, render.Overlay{Kind: render.FillOverlay, Bounds: r, Color: col, Radius: 2 + 2*pulse})
	}
	return overlays
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
	block, centered := hit.obj != nil && hit.obj.block, hit.obj != nil && hit.obj.alignCenter
	items := media.ImageToolButtons(box, e.lk.size, block, centered)
	overlays := make([]render.Overlay, 0, len(items)*3+2)
	for _, item := range items {
		overlays = append(overlays,
			render.Overlay{Kind: render.FillOverlay, Bounds: item.Bounds, Color: e.lk.panel, Radius: 5},
			render.Overlay{Kind: render.StrokeOverlay, Bounds: item.Bounds, Color: e.lk.border, Radius: 5, Width: 1},
		)
		sp := ui.Span{Text: item.Label, Size: e.lk.size * 0.7, Color: e.lk.text}
		w, h := p.MeasureText(item.Bounds.W-4, sp)
		overlays = append(overlays, render.Overlay{Kind: render.TextOverlay, Bounds: ui.Rect{X: item.Bounds.X + (item.Bounds.W-w)/2, Y: item.Bounds.Y + (item.Bounds.H-h)/2}, MaxW: item.Bounds.W - 4, Text: sp})
	}
	_, _, _, grip := media.ToolRects(box, e.lk.size)
	// A quiet L mark at the lower-right corner is the image resize affordance.
	col := e.lk.text.Alpha(0.72)
	overlays = append(overlays,
		render.Overlay{Kind: render.FillOverlay, Bounds: ui.Rect{X: grip.X + grip.W - 3, Y: grip.Y + grip.H*.42, W: 2, H: grip.H * .48}, Color: col, Radius: 1},
		render.Overlay{Kind: render.FillOverlay, Bounds: ui.Rect{X: grip.X + grip.W*.42, Y: grip.Y + grip.H - 3, W: grip.W * .48, H: 2}, Color: col, Radius: 1},
	)
	for _, overlay := range overlays {
		e.addRenderOverlay(overlay, true)
	}
}

// paintObject draws an image or a formula in r, or a placeholder for an
// image not loaded.
func (e *Editor) paintObject(p *ui.Painter, o *object, r ui.Rect) {
	o.refreshAssetBitmap()
	if o.embedBlocked != "" {
		block := render.Block{Kind: render.EmbedBlock, Bounds: r, Label: o.embedBlocked}
		e.renderScratch.addBlock(block)
		return
	}
	if o.embed != nil {
		resources := &e.renderScratch.resources
		block := render.Block{Kind: render.EmbedBlock, Bounds: r, Children: o.embed.visibleRenderBlocks(resources)}
		e.renderScratch.addBlock(block)
		return
	}
	if o.footnote {
		sp := ui.Span{Text: o.alt, Size: e.lk.size * 0.78, Color: e.lk.accent}
		ascent := ui.Font{Family: e.lk.font, Size: sp.Size}.Metrics().Ascent
		e.addRenderOverlay(render.Overlay{Kind: render.TextOverlay, Bounds: ui.Rect{X: r.X, Y: r.Y - ascent - e.lk.size*.4}, MaxW: r.W, Text: sp}, true)
		return
	}
	if o.math != nil {
		style := e.renderStyle()
		block := render.Block{Kind: render.MathBlock, Bounds: r, Resource: e.renderScratch.resources.addMath(o.math, style.Dark, style.Font, e.mathRenderer)}
		e.renderScratch.addBlock(block)
		return
	}
	if o.bmp != nil {
		block := render.Block{Kind: render.ImageBlock, Bounds: r, Resource: e.renderScratch.resources.addImage(o.bmp), Alt: o.alt}
		e.renderScratch.addBlock(block)
		return
	}
	e.addRenderOverlay(render.Overlay{Kind: render.FillOverlay, Bounds: r, Color: e.lk.panel, Radius: 6}, false)
	if o.imagePending {
		label := o.alt
		if label == "" {
			label = locales.EditorImageLoading()
		}
		sp := ui.Span{Text: label, Size: e.lk.size * 0.8, Color: e.lk.muted}
		pad := e.lk.size * 0.8
		textW := max(r.W-2*pad, 0)
		_, h := p.MeasureText(textW, sp)
		e.addRenderOverlay(render.Overlay{Kind: render.TextOverlay, Bounds: ui.Rect{X: r.X + pad, Y: r.Y + (r.H-h)/2}, Clip: r, ClipRadius: 6, MaxW: textW, Text: sp}, true)
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
	e.addRenderOverlay(render.Overlay{Kind: render.TextOverlay, Bounds: ui.Rect{X: r.X + pad, Y: r.Y + (r.H-h)/2}, Clip: r, ClipRadius: 6, MaxW: textW, Text: sp}, true)
}

// paintSourceChip draws a block's source control: the little `</>` button
// that shows the block as the Markdown it is written as.
func (e *Editor) paintSourceChip(p *ui.Painter, l *leaf, ox, oy float32) {
	r := e.sourceChip(l)
	r.X += ox
	r.Y += oy
	sp := ui.Span{Text: "</>", Size: e.lk.size * 0.72, Color: e.lk.muted}
	w, h := p.MeasureText(r.W-4, sp)
	overlays := []render.Overlay{
		{Kind: render.FillOverlay, Bounds: r, Color: e.lk.panel, Radius: 5},
		{Kind: render.StrokeOverlay, Bounds: r, Color: e.lk.border, Radius: 5, Width: 1},
		{Kind: render.TextOverlay, Bounds: ui.Rect{X: r.X + (r.W-w)/2, Y: r.Y + (r.H-h)/2}, MaxW: r.W - 4, Text: sp},
	}
	for _, overlay := range overlays {
		e.addRenderOverlay(overlay, true)
	}
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
	overlays := make([]render.Overlay, 0, len(l.firstOf))
	for _, k := range l.firstOf {
		c := l.conts[k]
		if (c.kind == cItem || c.kind == cFootnote) && e.containerPrefixOpen(l) {
			continue
		}
		x0 := e.colX + e.indentOf(l.conts[:k])
		w := e.indentOf(l.conts[k : k+1])
		switch c.kind {
		case cItem:
			overlays = append(overlays, e.itemMarkerOverlays(p, c, ox+x0, oy+baseline, w)...)
		case cFootnote:
			sp := ui.Span{Text: c.label + ":", Size: e.lk.size, Color: e.lk.accent}
			fm := ui.Font{Family: e.lk.font, Size: sp.Size}.Metrics()
			overlays = append(overlays, render.Overlay{Kind: render.TextOverlay, Bounds: ui.Rect{X: ox + x0, Y: oy + baseline - fm.Ascent}, MaxW: w, Text: sp})
		}
	}
	if len(overlays) > 0 {
		for _, overlay := range overlays {
			e.addRenderOverlay(overlay, true)
		}
	}
}

// paintItemMarker draws an item's marker in the column from x, w wide, on
// the baseline.
func (e *Editor) itemMarkerOverlays(p *ui.Painter, c cont, x, baseline, w float32) []render.Overlay {
	s := e.lk.size
	if c.task != syntax.IncrementalNoTask {
		box := s * 0.95
		r := ui.Rect{X: x + w - box - s*0.45, Y: baseline - s*0.82, W: box, H: box}
		if c.task == syntax.IncrementalChecked {
			color := theme.From(e.lk.accent, e.lk.dark).OnAccent
			return []render.Overlay{
				{Kind: render.FillOverlay, Bounds: r, Color: e.lk.accent, Radius: 3.5},
				{Kind: render.PolylineOverlay, Points: [][2]float32{{r.X + r.W*.24, r.Y + r.H*.52}, {r.X + r.W*.43, r.Y + r.H*.7}, {r.X + r.W*.77, r.Y + r.H*.32}}, Width: 1.8, Color: color},
			}
		}
		return []render.Overlay{{Kind: render.StrokeOverlay, Bounds: r, Color: e.lk.muted.Alpha(0.8), Radius: 3.5, Width: 1.3}}
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
	return []render.Overlay{{Kind: render.TextOverlay, Bounds: ui.Rect{X: x + w - tw - s*.45, Y: baseline - fm.Ascent}, Text: sp}}
}

// paintTable draws a table's grid and its header row's fill.
func (e *Editor) paintTable(p *ui.Painter, l *leaf, ox, oy float32, t *ui.Theme) {
	g := l.geom
	if g == nil || len(g.rowY) < 2 || len(g.colX) < 2 {
		return
	}
	x, y := ox+tableContentX(l), oy+l.y
	h := g.rowY[len(g.rowY)-1]
	box := ui.Rect{X: x, Y: y, W: g.w, H: h}
	const radius = 4
	line := uiMarkdownColor(theme.MarkdownColors(e.lk.dark).TableRule)
	overlays := make([]render.Overlay, 0, len(g.rowY)+len(g.colX)+3)
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
				top, left, bottom, right := tableRegionBounds(r)
				top = max(0, min(top, len(g.rowY)-2))
				bottom = max(top, min(bottom, len(g.rowY)-2))
				left = max(0, min(left, len(g.colX)-2))
				right = max(left, min(right, len(g.colX)-2))
				selected = ui.Rect{X: x + g.colX[left], Y: y + g.rowY[top], W: g.colX[right+1] - g.colX[left], H: g.rowY[bottom+1] - g.rowY[top]}
			}
		case tableRows:
			first, last := clampPaintSelection(s.first, s.last, len(g.rowY)-1)
			selected.Y = y + g.rowY[first]
			selected.H = g.rowY[last+1] - g.rowY[first]
		case tableColumns:
			first, last := clampPaintSelection(s.first, s.last, len(g.colX)-1)
			selected.X = x + g.colX[first]
			selected.W = g.colX[last+1] - g.colX[first]
		}
		if s.axis == tableCells {
			clip := ui.Rect{X: ox + l.x, Y: y, W: tableViewportWidth(l), H: h}
			overlays = append(overlays,
				render.Overlay{Kind: render.FillOverlay, Bounds: selected, Clip: clip, Color: selection, Radius: radius},
				render.Overlay{Kind: render.StrokeOverlay, Bounds: selected, Clip: clip, Color: e.lk.accent.Alpha(0.6), Radius: radius, Width: 1.5},
			)
		} else {
			overlays = append(overlays,
				render.Overlay{Kind: render.FillOverlay, Bounds: selected, Color: selection, Radius: radius},
				render.Overlay{Kind: render.StrokeOverlay, Bounds: selected, Color: e.lk.accent.Alpha(0.6), Radius: radius, Width: 1.5},
			)
		}
	}
	for i, ry := range g.rowY {
		if i == 0 || i == len(g.rowY)-1 {
			continue
		}
		overlays = append(overlays, render.Overlay{Kind: render.FillOverlay, Bounds: ui.Rect{X: x, Y: y + ry - .5, W: g.w, H: 1}, Color: line})
	}
	for i, cx := range g.colX {
		if i == 0 || i == len(g.colX)-1 {
			continue
		}
		overlays = append(overlays, render.Overlay{Kind: render.FillOverlay, Bounds: ui.Rect{X: x + cx - .5, Y: y, W: 1, H: h}, Color: line})
	}
	overlays = append(overlays, render.Overlay{Kind: render.StrokeOverlay, Bounds: box, Color: line, Radius: radius, Width: 1})
	for _, overlay := range overlays {
		e.addRenderOverlay(overlay, false)
	}
}

// clampPaintSelection keeps a structurally selected range inside the current
// layout. A document transaction can temporarily shrink a table while a
// visual delete/paste sequence still owns the previous range.
func clampPaintSelection(first, last, count int) (int, int) {
	if count <= 0 {
		return 0, 0
	}
	first = max(0, min(first, count-1))
	last = max(first, min(last, count-1))
	return first, last
}

// paintCaret draws the caret, blinking, while the editor has the focus.
func (e *Editor) paintCaret(p *ui.Painter, ox, oy float32, clip ui.Rect) {
	if e.compositionText() == "" && e.tableBridge == nil {
		if rowCaret, ok := e.tableSelectionCaretRect(); ok {
			// Structural row selection is its own focus state. Keep the active
			// boundary visible even if the text editor has temporarily lost focus.
			e.paintTableBoundaryCaret(p, ox, oy, rowCaret, clip)
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
	if e.compositionText() == "" {
		if _, _, _, boundary := e.tableBoundaryAtOffset(e.Caret()); boundary {
			if r, ok := e.caretRect(e.Caret()); ok {
				e.paintTableBoundaryCaret(p, ox, oy, r, clip)
				return
			}
		} else if t, _ := e.tableAtEdge(e.Caret()); t != nil {
			if r, ok := e.caretRect(e.Caret()); ok {
				e.paintTableBoundaryCaret(p, ox, oy, r, clip)
				return
			}
		}
	}
	phase := p.Now().Sub(e.blinkStart)
	if phase < 30*time.Second {
		untilTransition := feedback.CaretBlinkPeriod - phase%feedback.CaretBlinkPeriod
		if e.opts.CursorFade {
			if _, fading := feedback.CaretFadeOpacity(phase); fading {
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
	if e.opts.CursorFade && e.compositionText() == "" {
		caretAlpha, _ = feedback.CaretFadeOpacity(phase)
	} else if e.compositionText() == "" && phase%(2*feedback.CaretBlinkPeriod) >= feedback.CaretBlinkPeriod {
		return
	}
	if e.Session.Selection().Anchor != e.Caret() && e.compositionText() == "" && !e.blockCaret() {
		return
	}
	r, ok := e.composeCaretRect()
	if !ok {
		r, ok = e.caretRect(e.Caret())
	}
	if !ok {
		return
	}
	transition := e.feedbackState.CursorState()
	if e.compositionText() == "" && feedback.SmoothCursorNeedsFrames(e.opts.SmoothCursor, transition.Active) {
		from := ui.Rect{X: transition.FromRect.X, Y: transition.FromRect.Y, W: transition.FromRect.W, H: transition.FromRect.H}
		fromOK := transition.FromRectSet
		if !fromOK {
			from, fromOK = e.caretRect(transition.From)
		}
		if fromOK {
			target := r
			e.feedbackState.SetCursorRects(
				feedback.Rect{X: from.X, Y: from.Y, W: from.W, H: from.H},
				feedback.Rect{X: target.X, Y: target.Y, W: target.W, H: target.H},
			)
			if visual, active := e.feedbackState.CursorRects(p.Now()); active {
				r = ui.Rect{X: visual.X, Y: visual.Y, W: visual.W, H: visual.H}
				p.AnimationFrameRate(feedback.SmoothCursorFrameRate)
			}
		} else {
			e.feedbackState.CancelCursor()
		}
	}
	if e.blockCaret() && e.compositionText() == "" {
		r = e.blockCaretRect(r)
		box := ui.Rect{X: ox + r.X, Y: oy + r.Y, W: r.W, H: r.H}
		e.paintCaretOverlay(p, clip, render.Overlay{Kind: render.FillOverlay, Bounds: box, Color: e.lk.caret.Alpha(caretAlpha)})
		// Repaint the shaped text through the caret box so the glyph itself
		// takes a contrasting foreground on the opaque caret background.
		if us := e.units(); len(us) > 0 {
			if i := e.unitIndex(us, e.Caret()); i >= 0 {
				u := us[i]
				v := e.unitView(u, e.Caret())
				if v.layout != nil {
					if d, ok := v.disp.srcToDisp(e.Caret() - u.lineStart); ok {
						if sp, ok := blockCaretGlyph(v.disp, d, blockCaretTextColor(e.lk.caret).Alpha(caretAlpha)); ok {
							glyph := ui.ShapeRichText([]ui.Span{sp}, v.font, 0)
							lines, glyphLines := v.layout.Lines(), glyph.Lines()
							if len(lines) > 0 && len(glyphLines) > 0 {
								ln := lines[lineOf(lines, d, e.upstream)]
								y := oy + u.y + v.ty + ln.Baseline - glyphLines[0].Baseline
								e.paintCaretOverlay(p, clip, render.Overlay{Kind: render.LayoutOverlay, Clip: box, ClipRadius: 0, Layout: &textlayout.Layout{TextLayout: glyph}, X: box.X, Y: y, Color: blockCaretTextColor(e.lk.caret).Alpha(caretAlpha)})
							}
						}
					}
				}
			}
		}
		return
	}
	h := r.H
	e.paintCaretOverlay(p, clip, render.Overlay{Kind: render.FillOverlay, Bounds: ui.Rect{X: ox + r.X - .75, Y: oy + r.Y, W: 1.75, H: h}, Color: e.lk.caret.Alpha(caretAlpha), Radius: .5})
}

func (e *Editor) paintCaretOverlay(p *ui.Painter, clip ui.Rect, overlay render.Overlay) {
	if clip.W > 0 && clip.H > 0 {
		if overlay.Clip.W > 0 && overlay.Clip.H > 0 {
			x1, y1 := max(clip.X, overlay.Clip.X), max(clip.Y, overlay.Clip.Y)
			x2, y2 := min(clip.X+clip.W, overlay.Clip.X+overlay.Clip.W), min(clip.Y+clip.H, overlay.Clip.Y+overlay.Clip.H)
			if x2 <= x1 || y2 <= y1 {
				return
			}
			overlay.Clip = ui.Rect{X: x1, Y: y1, W: x2 - x1, H: y2 - y1}
		} else {
			overlay.Clip = clip
		}
	}
	e.renderScratch.addOverlay(overlay, true)
}

func (e *Editor) paintTableBoundaryCaret(p *ui.Painter, ox, oy float32, r, clip ui.Rect) {
	// A table boundary has no character cell to invert. Use the same thin
	// accent stroke for every side and row, independent of the modal mode.
	e.paintCaretOverlay(p, clip, render.Overlay{Kind: render.FillOverlay, Bounds: ui.Rect{X: ox + r.X - .75, Y: oy + r.Y, W: 2.25, H: r.H}, Color: e.lk.caret, Radius: .5})
}

// composeCaretRect returns the caret's box inside the composition of an
// input method.
func (e *Editor) composeCaretRect() (ui.Rect, bool) {
	if e.compositionText() == "" {
		return ui.Rect{}, false
	}
	us := e.units()
	i := e.unitIndex(us, e.Caret())
	if i < 0 {
		return ui.Rect{}, false
	}
	u := us[i]
	v := u.view
	if v == nil || v.layout == nil {
		return ui.Rect{}, false
	}
	for _, pc := range v.disp.pieces {
		if pc.kind == pDeco && pc.text == e.compositionText() && pc.s0+u.lineStart == e.Caret() {
			r := v.layout.Caret(pc.d0 + min(e.compositionSelection().End, pc.d1-pc.d0))
			r.X += u.x + v.tx
			r.Y += u.y + v.ty
			return r, true
		}
	}
	return ui.Rect{}, false
}
