package editor

import (
	"github.com/egoist/mygo/ui"
	render "muse/nativeui/editor/render"
)

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
	blocks := render.ProjectCallouts(e.calloutLines(visible), top, bottom)
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
