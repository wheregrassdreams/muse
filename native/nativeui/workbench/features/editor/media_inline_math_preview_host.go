package editor

import (
	"github.com/egoist/mygo/ui"
	texmath "muse/markdown/math"
	media "muse/nativeui/workbench/features/editor/blocks/media"
)

// InlineMathPreview is the editor host seam for the media-owned preview
// behavior. The adapter exposes values and narrow operations only.
func (e *Editor) InlineMathPreview() (media.MathPreviewFrame, bool) {
	if e == nil {
		return media.MathPreviewFrame{}, false
	}
	return media.InlineMathPreview(editorInlineMathHost{editor: e})
}

type editorInlineMathHost struct{ editor *Editor }

func (h editorInlineMathHost) PreviewAllowed() bool {
	e := h.editor
	return e.focused && !e.isComposing() && !e.inlineMath.PreviewHidden()
}

func (h editorInlineMathHost) ActiveFormula() (int, string, bool) {
	start, _, tex, ok := h.editor.activeInlineMath()
	return start, tex, ok
}

func (h editorInlineMathHost) CaretRect(offset int) (ui.Rect, bool) {
	return h.editor.caretRect(offset)
}

func (h editorInlineMathHost) CaretOffset() int { return h.editor.Caret() }

func (h editorInlineMathHost) Viewport() (media.Viewport, float32) {
	e := h.editor
	return media.Viewport{X: e.viewport.X, Y: e.viewport.Y, W: e.viewport.W, H: e.viewport.H}, e.scroll.Y
}

func (h editorInlineMathHost) TextStyle() (float32, string, bool) {
	e := h.editor
	return e.lk.size, e.lk.font, e.lk.dark
}

func (h editorInlineMathHost) Typeset(tex string, fontSize float32) (*texmath.Raster, float32, float32, float32, bool) {
	obj, ok := media.TypesetInline(tex, fontSize, false)
	if !ok || obj.Raster == nil {
		return nil, 0, 0, 0, false
	}
	return obj.Raster, obj.Width, obj.Height, obj.Descent, true
}
