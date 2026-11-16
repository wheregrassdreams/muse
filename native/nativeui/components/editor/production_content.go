package editor

import (
	"github.com/egoist/mygo/ui"

	"muse/editor"
	"muse/markdown/diagrams"
	markdownui "muse/nativeui/components/markdown"
	"muse/nativeui/window"
)

const diagramPainterCacheLimit = 8

type diagramPainterCacheEntry struct {
	model   *diagrams.Model
	painter markdownui.Diagram
	used    uint64
}

// ProductionContent is a temporary migration bridge from the production
// editor's existing virtualized view to the generic pane lifecycle. It keeps
// existing behavior while toolkit input/render responsibilities move into
// this component; it is not the final editor boundary because Editor.View
// still owns MyGo work.
type ProductionContent struct {
	editor   *editor.Editor
	onClose  func()
	closed   bool
	diagrams []diagramPainterCacheEntry
	clock    uint64
	rail     headingRailState
}

// NewProductionContent hosts the existing production editor as pane content.
// This bridge should be removed when the migrated Session-backed component
// owns the production rendering and input path.
func NewProductionContent(value *editor.Editor, onClose func()) *ProductionContent {
	content := &ProductionContent{editor: value, onClose: onClose, rail: headingRailState{hover: -1}}
	if value != nil {
		value.SetDiagramRenderer(content.drawDiagram)
		value.SetMathRenderer(drawMathRaster)
	}
	return content
}

func (c *ProductionContent) Render(ctx *ui.Context) {
	if c == nil || c.closed || c.editor == nil {
		return
	}
	c.editor.ViewWithInput(ctx, c.handleInput)
	viewport := c.editor.ViewportBounds()
	bounds := ui.Rect{X: viewport.X, Y: viewport.Y, W: viewport.W, H: viewport.H}
	frame := c.editor.HeadingRailSnapshot(editor.HeadingRailViewport{Width: viewport.W, Height: viewport.H})
	c.drawHeadingRail(ctx, bounds, frame, c.editor.HeadingRailPreview, func(offset int) { c.editor.JumpToHeading(offset) })
	if frame, ok := c.editor.InlineMathPreview(); ok {
		drawInlineMathPreview(ctx, frame)
	}
}

// handleInput owns the MyGo-to-editor input translation for the production
// host. Pointer/IME/drop events still delegate to the old adapter until
// their typed editor operations are migrated.
func (c *ProductionContent) handleInput(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputKeyDown:
		key, ok := keyEventFromUI(ev)
		return ok && c.editor.DispatchKey(key)
	case ui.InputText:
		result, err := c.editor.Session.Dispatch(editor.InputEvent{Kind: editor.InputInsertText, Text: ev.Text})
		return err == nil && result.Handled
	default:
		return c.editor.HandleNativeInput(ev)
	}
}

func (c *ProductionContent) drawDiagram(p *ui.Painter, rect ui.Rect, theme *ui.Theme, model *diagrams.Model) {
	if model == nil {
		return
	}
	c.clock++
	for i := range c.diagrams {
		entry := &c.diagrams[i]
		if entry.model == model {
			entry.used = c.clock
			entry.painter.Draw(p, rect, theme)
			return
		}
	}
	painter := markdownui.DiagramFromModel(*model)
	if len(c.diagrams) < diagramPainterCacheLimit {
		c.diagrams = append(c.diagrams, diagramPainterCacheEntry{model: model, painter: painter, used: c.clock})
		painter.Draw(p, rect, theme)
		return
	}
	lru := 0
	for i := range c.diagrams {
		if c.diagrams[i].used < c.diagrams[lru].used {
			lru = i
		}
	}
	c.diagrams[lru] = diagramPainterCacheEntry{model: model, painter: painter, used: c.clock}
	painter.Draw(p, rect, theme)
}

func (c *ProductionContent) Close() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	if c.editor != nil {
		c.editor.SetDiagramRenderer(nil)
		c.editor.SetMathRenderer(nil)
	}
	c.diagrams = nil
	c.rail.ys = nil
	c.rail.scales = nil
	c.rail.rect = ui.Rect{}
	if c.onClose != nil {
		c.onClose()
	}
}

var _ window.Content = (*ProductionContent)(nil)
