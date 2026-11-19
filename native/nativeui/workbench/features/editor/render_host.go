package editor

import (
	"github.com/egoist/mygo/ui"
	texmath "muse/markdown/math"
	media "muse/nativeui/workbench/features/editor/blocks/media"
	render "muse/nativeui/workbench/features/editor/render"
	markdownui "muse/nativeui/workbench/framework/markdown"
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
	blocks    []render.Block
	behind    []render.Overlay
	inFront   []render.Overlay
	clip      ui.Rect
	resources editorRenderResources
}

func (s *editorRenderScratch) reset() {
	s.blocks = s.blocks[:0]
	s.behind = s.behind[:0]
	s.inFront = s.inFront[:0]
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
	return render.Snapshot{
		Revision: e.Session.Revision(), Viewport: viewport, Style: style,
		Behind: s.behind, Blocks: s.blocks, InFront: s.inFront,
	}
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
