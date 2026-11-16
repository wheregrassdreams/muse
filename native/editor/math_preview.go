package editor

import texmath "muse/markdown/math"

// EditorViewport is backend-neutral editor viewport geometry in logical pixels.
type EditorViewport struct {
	X, Y, W, H float32
}

// MathPreviewFrame is a detached view of the active inline formula. The raster
// is immutable and revision-independent; the host owns all drawing and card
// interaction. Width and Height are the formula's natural logical size.
type MathPreviewFrame struct {
	Viewport EditorViewport
	Anchor   EditorViewport
	Raster   *texmath.Raster
	Width    float32
	Height   float32
	FontSize float32
	Font     string
	Dark     bool
	Failed   bool
}

// InlineMathPreview returns the current formula preview if one is active and
// its anchor is visible. It has no document or selection side effects.
func (e *Editor) InlineMathPreview() (MathPreviewFrame, bool) {
	if !e.focused || e.composing || e.inlineMath.PreviewHidden() {
		return MathPreviewFrame{}, false
	}
	start, _, tex, active := e.activeInlineMath()
	if !active {
		return MathPreviewFrame{}, false
	}
	anchor, ok := e.mathPreviewAnchor(start)
	if !ok {
		return MathPreviewFrame{}, false
	}
	frame := MathPreviewFrame{
		Viewport: e.viewport,
		Anchor:   anchor,
		FontSize: e.lk.size,
		Font:     e.lk.font,
		Dark:     e.lk.dark,
	}
	obj := e.mathObject(tex, e.lk.size, false)
	if obj == nil {
		frame.Width, frame.Height, frame.Failed = 240, 48, true
		return frame, true
	}
	frame.Raster = obj.math
	frame.Width, frame.Height = obj.w, obj.h+obj.descent
	return frame, true
}

func (e *Editor) mathPreviewAnchor(start int) (EditorViewport, bool) {
	anchor, ok := e.caretRect(start)
	if !ok {
		return EditorViewport{}, false
	}
	anchor.X += e.viewport.X
	anchor.Y += e.viewport.Y - e.scroll.Y
	// Keep the preview associated with the formula's opening delimiter while
	// it is visible. If the formula wraps beyond the viewport, follow the live
	// caret so its preview stays available during editing.
	if anchor.Y+anchor.H <= e.viewport.Y || anchor.Y >= e.viewport.Y+e.viewport.H {
		anchor, ok = e.caretRect(e.caret)
		if !ok {
			return EditorViewport{}, false
		}
		anchor.X += e.viewport.X
		anchor.Y += e.viewport.Y - e.scroll.Y
	}
	return EditorViewport{X: anchor.X, Y: anchor.Y, W: anchor.W, H: anchor.H}, true
}
