package editor

import (
	"github.com/egoist/mygo/ui"

	"muse/markdown/diagrams"
	media "muse/nativeui/components/editor/blocks/media"
	"muse/nativeui/components/editor/navigationrail"
	markdownui "muse/nativeui/components/markdown"
	"muse/nativeui/window"
)

const diagramPainterCacheLimit = 8

type diagramPainterCacheEntry struct {
	model   *diagrams.Model
	painter markdownui.Diagram
	used    uint64
}

// Render makes the production editor a generic pane content surface. The
// Editor remains the sole owner of its virtualized render and input state.
func (e *Editor) Render(ctx *ui.Context) {
	if e == nil || e.closed {
		return
	}
	e.View(ctx)
	viewport := e.ViewportBounds()
	bounds := ui.Rect{X: viewport.X, Y: viewport.Y, W: viewport.W, H: viewport.H}
	frame := e.HeadingRailSnapshot(navigationrail.ViewportSize{Width: viewport.W, Height: viewport.H})
	e.rail.Draw(ctx, bounds, frame, e.HeadingRailPreview, func(offset int) { e.JumpToHeading(offset) })
	if frame, ok := e.InlineMathPreview(); ok {
		media.DrawInlineMathPreview(ctx, frame)
	}
}

// diagramVisual returns the bounded cached painter for a neutral diagram
// model. The UI renderer borrows this capability for the current frame.
func (e *Editor) diagramVisual(model *diagrams.Model) markdownui.Diagram {
	if model == nil {
		return markdownui.Diagram{}
	}
	e.diagramClock++
	for i := range e.diagrams {
		entry := &e.diagrams[i]
		if entry.model == model {
			entry.used = e.diagramClock
			return entry.painter
		}
	}
	painter := markdownui.DiagramFromModel(*model)
	if len(e.diagrams) < diagramPainterCacheLimit {
		e.diagrams = append(e.diagrams, diagramPainterCacheEntry{model: model, painter: painter, used: e.diagramClock})
		return painter
	}
	lru := 0
	for i := range e.diagrams {
		if e.diagrams[i].used < e.diagrams[lru].used {
			lru = i
		}
	}
	e.diagrams[lru] = diagramPainterCacheEntry{model: model, painter: painter, used: e.diagramClock}
	return painter
}

func (e *Editor) Close() {
	if e == nil || e.closed {
		return
	}
	e.closed = true
	if e.Session != nil {
		e.Session.DetachPresentation()
	}
	e.SetMathRenderer(nil)
	e.diagrams = nil
	e.rail.Reset()
	if e.opts.OnClose != nil {
		e.opts.OnClose()
	}
}

var _ window.Content = (*Editor)(nil)
