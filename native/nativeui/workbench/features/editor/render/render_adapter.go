package editor

import (
	"github.com/egoist/mygo/ui"
	texmath "muse/markdown/math"
	media "muse/nativeui/workbench/features/editor/blocks/media"
	render "muse/nativeui/workbench/features/editor/render"
	markdownui "muse/nativeui/workbench/features/markdown"
)

// editorRenderResources is a frame-scoped set of visible assets. It stores
// only decoded presentation values and is discarded as soon as Paint returns.
type editorRenderResources struct {
	next         render.ResourceID
	images       map[render.ResourceID]*ui.Bitmap
	maths        map[render.ResourceID]render.MathVisual
	mathSources  map[render.ResourceID]*texmath.Raster
	mathPainters map[render.ResourceID]MathRenderer
	diagrams     map[render.ResourceID]markdownui.Diagram
}

// editorRenderScratch owns reusable frame and resource buffers. The host
// resets it under paintMu and render borrows the slices only for Paint.
type editorRenderScratch struct {
	blocks          []render.Block
	behind          []render.Overlay
	inFront         []render.Overlay
	revealRanges    []render.SourceRange
	highlightRanges []render.SourceRange
	clip            ui.Rect
	resources       editorRenderResources
}

func (s *editorRenderScratch) reset() {
	s.blocks = s.blocks[:0]
	s.behind = s.behind[:0]
	s.inFront = s.inFront[:0]
	s.revealRanges = s.revealRanges[:0]
	s.highlightRanges = s.highlightRanges[:0]
	s.clip = ui.Rect{}
	s.resources.reset()
}

func (s *editorRenderScratch) addBlock(block render.Block) {
	if s.clip.W > 0 && s.clip.H > 0 && block.Clip.W == 0 {
		block.Clip = s.clip
	}
	s.blocks = append(s.blocks, block)
}

func (s *editorRenderScratch) addOverlay(overlay render.Overlay, front bool) {
	if s.clip.W > 0 && s.clip.H > 0 {
		if overlay.Clip.W > 0 && overlay.Clip.H > 0 {
			x1, y1 := max(s.clip.X, overlay.Clip.X), max(s.clip.Y, overlay.Clip.Y)
			x2, y2 := min(s.clip.X+s.clip.W, overlay.Clip.X+overlay.Clip.W), min(s.clip.Y+s.clip.H, overlay.Clip.Y+overlay.Clip.H)
			if x2 <= x1 || y2 <= y1 {
				return
			}
			overlay.Clip = ui.Rect{X: x1, Y: y1, W: x2 - x1, H: y2 - y1}
		} else {
			overlay.Clip = s.clip
		}
	}
	if front {
		s.inFront = append(s.inFront, overlay)
	} else {
		s.behind = append(s.behind, overlay)
	}
}

func (s *editorRenderScratch) snapshot(e *Editor, viewport ui.Rect, style render.Style) render.Snapshot {
	selection := e.SelectionRange()
	s.revealRanges = appendRanges(s.revealRanges[:0], e.sourceRevealRanges)
	s.highlightRanges = appendRanges(s.highlightRanges[:0], e.highlights)
	return render.Snapshot{
		Revision: e.Session.Revision(), Source: e.Source(), Caret: e.Caret(),
		Selection: render.SourceRange{Start: selection.Start, End: selection.End}, HasSelection: selection.Start != selection.End,
		Reveal: s.revealRanges, Highlights: s.highlightRanges,
		Viewport: viewport, Style: style,
		Options: render.ProjectionOptions{SmartTypography: !e.opts.DisableSmartTypography, SoftBreaksAsSpaces: e.opts.SoftBreaksAsSpaces},
		Behind:  s.behind, Blocks: s.blocks, InFront: s.inFront,
	}
}

func appendRanges(dst []render.SourceRange, src []Range) []render.SourceRange {
	for _, r := range src {
		dst = append(dst, render.SourceRange{Start: r.Start, End: r.End})
	}
	return dst
}

func (e *Editor) addRenderOverlay(overlay render.Overlay, front bool) {
	e.renderScratch.addOverlay(overlay, front)
}

func (r *editorRenderResources) reset() {
	r.next = 0
	if r.images != nil {
		clear(r.images)
	} else {
		r.images = make(map[render.ResourceID]*ui.Bitmap)
	}
	if r.maths != nil {
		clear(r.maths)
	} else {
		r.maths = make(map[render.ResourceID]render.MathVisual)
	}
	if r.mathSources != nil {
		clear(r.mathSources)
	} else {
		r.mathSources = make(map[render.ResourceID]*texmath.Raster)
	}
	if r.mathPainters != nil {
		clear(r.mathPainters)
	} else {
		r.mathPainters = make(map[render.ResourceID]MathRenderer)
	}
	if r.diagrams != nil {
		clear(r.diagrams)
	} else {
		r.diagrams = make(map[render.ResourceID]markdownui.Diagram)
	}
}

func (r *editorRenderResources) id() render.ResourceID {
	r.next++
	return r.next
}

func (r *editorRenderResources) addImage(bitmap *ui.Bitmap) render.ResourceID {
	id := r.id()
	r.images[id] = bitmap
	return id
}

func (r *editorRenderResources) addMath(raster *texmath.Raster, dark bool, font string, painter MathRenderer) render.ResourceID {
	id := r.id()
	if painter == nil {
		r.maths[id] = media.MathVisualFor(raster, dark, font)
	}
	r.mathSources[id] = raster
	r.mathPainters[id] = painter
	return id
}

func (r *editorRenderResources) addDiagram(painter markdownui.Diagram) render.ResourceID {
	id := r.id()
	r.diagrams[id] = painter
	return id
}

func (r *editorRenderResources) Image(id render.ResourceID) (*ui.Bitmap, bool) {
	bitmap, ok := r.images[id]
	return bitmap, ok && bitmap != nil
}

func (r *editorRenderResources) Math(id render.ResourceID, _ bool, _ string) (render.MathVisual, bool) {
	visual, ok := r.maths[id]
	return visual, ok && visual != nil
}

func (r *editorRenderResources) DrawMath(p *ui.Painter, rect ui.Rect, id render.ResourceID, dark bool, font string) bool {
	if painter, source := r.mathPainters[id], r.mathSources[id]; painter != nil && source != nil {
		painter(p, rect, source, dark, font)
		return true
	}
	return false
}

func (r *editorRenderResources) Diagram(id render.ResourceID) (render.DiagramVisual, bool) {
	painter, ok := r.diagrams[id]
	return painter, ok
}

func (r *editorRenderResources) ports() render.Resources {
	return render.Resources{Images: r, Math: r, Diagrams: r}
}

func (e *Editor) visibleRenderBlocks(resources *editorRenderResources) []render.Block {
	if e == nil || resources == nil {
		return nil
	}
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	e.layoutIfNeeded()
	top := e.scroll.Y - 40
	bottom := e.scroll.Y + e.viewH + 40
	if e.viewH <= 0 {
		bottom = e.docH
	}
	visible := e.paintVisibleSpan(top, bottom)
	blocks := e.calloutRenderBlocks(visible)
	for _, leaf := range visible {
		if leaf.kind == kTable && leaf.tab != nil {
			for _, row := range leaf.tab.rows {
				for _, cell := range row {
					blocks = append(blocks, e.visibleRenderBlock(cell, resources)...)
				}
			}
			continue
		}
		blocks = append(blocks, e.visibleRenderBlock(leaf, resources)...)
	}
	return blocks
}

func (e *Editor) calloutRenderBlocks(visible []*leaf) []render.Block {
	type card struct {
		c       cont
		x, y, z float32
	}
	byID := make(map[int]*card)
	var cards []*card
	for _, leaf := range visible {
		x := e.colX
		for _, container := range leaf.conts {
			if isCalloutContainer(container) {
				item := byID[container.id]
				if item == nil {
					item = &card{c: container, x: x, y: leaf.y - e.lk.size*.45, z: leaf.y + leaf.h}
					byID[container.id] = item
					cards = append(cards, item)
				}
				item.z = leaf.y + leaf.h
			}
			x += e.indentOf([]cont{container})
		}
	}
	blocks := make([]render.Block, 0, len(cards))
	for _, item := range cards {
		if item.z < 0 || item.y > e.docH {
			continue
		}
		title := item.c.title
		header := &leaf{lineStart: lineStartAt(e.Source(), item.c.start), conts: []cont{item.c}}
		if e.calloutHeaderSourceOpen(header) {
			title = ""
		}
		blocks = append(blocks, render.Block{
			Kind:   render.CalloutBlock,
			Bounds: ui.Rect{X: item.x, Y: item.y, W: e.colW - (item.x - e.colX), H: item.z - item.y + e.lk.size*.6},
			Title:  title, Alert: renderAlertKind(item.c.alert),
		})
	}
	return blocks
}

func (e *Editor) visibleRenderBlock(leaf *leaf, resources *editorRenderResources) []render.Block {
	if leaf == nil {
		return nil
	}
	var blocks []render.Block
	view := leaf.view
	if view == nil {
		return nil
	}
	if view.layout != nil && view.diagramModel == nil {
		w, h := view.layout.Size()
		blocks = append(blocks, render.Block{
			Kind:   render.TextBlock,
			Bounds: ui.Rect{X: leaf.x, Y: leaf.y, W: max(w, leaf.w), H: max(h, leaf.h)},
			Layout: view.layout, TextX: leaf.x - blockScrollOffset(leaf) + view.tx, TextY: leaf.y + view.ty,
		})
	}
	if view.block != nil {
		object := view.block
		object.refreshAssetBitmap()
		bounds := ui.Rect{X: leaf.x - blockScrollOffset(leaf) + view.bx, Y: leaf.y + view.by, W: object.w, H: object.h}
		switch {
		case object.embed != nil:
			children := object.embed.visibleRenderBlocks(resources)
			blocks = append(blocks, render.Block{Kind: render.EmbedBlock, Bounds: bounds, Children: children, Label: object.embedBlocked})
		case object.bmp != nil:
			blocks = append(blocks, render.Block{Kind: render.ImageBlock, Bounds: bounds, Resource: resources.addImage(object.bmp), Alt: object.alt})
		case object.math != nil:
			blocks = append(blocks, render.Block{Kind: render.MathBlock, Bounds: bounds, Resource: resources.addMath(object.math, e.lk.dark, e.lk.font, e.mathRenderer)})
		case object.embedBlocked != "":
			blocks = append(blocks, render.Block{Kind: render.EmbedBlock, Bounds: bounds, Label: object.embedBlocked})
		}
	}
	if view.diagramModel != nil {
		box, _ := e.diagramBox(leaf, view, 0, 0)
		blocks = append(blocks, render.Block{Kind: render.DiagramBlock, Bounds: box, Resource: resources.addDiagram(e.diagramVisual(view.diagramModel))})
	}
	return blocks
}

func (e *Editor) renderStyle() render.Style {
	return render.Style{
		Dark: e.lk.dark, Text: e.lk.text, Muted: e.lk.muted, Accent: e.lk.accent,
		Panel: e.lk.panel, Border: e.lk.border, Selection: e.lk.selection,
		Caret: e.lk.caret, QuoteBar: e.lk.quoteBar, Font: e.lk.font,
		Mono: e.lk.mono, FontSize: e.lk.size, LineHeight: e.lk.lh,
		Heading: e.lk.heading, Marker: e.lk.marker, Link: e.lk.link, LinkHover: e.lk.linkHover,
		InlineCodeText: e.lk.codeText, InlineCodeBackground: e.lk.codeBg, MarkBackground: e.lk.markBg,
	}
}
