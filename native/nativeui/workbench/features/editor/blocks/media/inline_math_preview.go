package media

import (
	"github.com/egoist/mygo/ui"
	texmath "muse/markdown/math"
)

// InlineMathHost exposes the small set of live editor facts needed to build a
// detached inline-math preview. It does not expose parser or selection state.
type InlineMathHost interface {
	PreviewAllowed() bool
	ActiveFormula() (sourceOffset int, tex string, ok bool)
	CaretOffset() int
	CaretRect(sourceOffset int) (ui.Rect, bool)
	Viewport() (Viewport, float32)
	TextStyle() (fontSize float32, font string, dark bool)
	Typeset(tex string, fontSize float32) (raster *texmath.Raster, width, height, descent float32, ok bool)
}

// InlineMathPreview builds the preview frame from a short-lived host view.
// It owns anchor fallback and failure presentation policy; the host retains
// parser, caret, theme, and typesetting ownership.
func InlineMathPreview(host InlineMathHost) (MathPreviewFrame, bool) {
	if host == nil || !host.PreviewAllowed() {
		return MathPreviewFrame{}, false
	}
	start, tex, ok := host.ActiveFormula()
	if !ok {
		return MathPreviewFrame{}, false
	}
	viewport, scrollY := host.Viewport()
	anchor, ok := previewAnchor(host, viewport, scrollY, start)
	if !ok {
		return MathPreviewFrame{}, false
	}
	fontSize, font, dark := host.TextStyle()
	frame := MathPreviewFrame{Viewport: viewport, Anchor: anchor, FontSize: fontSize, Font: font, Dark: dark}
	raster, width, height, descent, rendered := host.Typeset(tex, fontSize)
	if !rendered || raster == nil {
		frame.Width, frame.Height, frame.Failed = 240, 48, true
		return frame, true
	}
	frame.Raster = raster
	frame.Width, frame.Height = width, height+descent
	return frame, true
}

func previewAnchor(host InlineMathHost, viewport Viewport, scrollY float32, sourceOffset int) (Viewport, bool) {
	anchor, ok := host.CaretRect(sourceOffset)
	if !ok {
		return Viewport{}, false
	}
	anchor.X += viewport.X
	anchor.Y += viewport.Y - scrollY
	if anchor.Y+anchor.H <= viewport.Y || anchor.Y >= viewport.Y+viewport.H {
		caret := host.CaretOffset()
		anchor, ok = host.CaretRect(caret)
		if !ok {
			return Viewport{}, false
		}
		anchor.X += viewport.X
		anchor.Y += viewport.Y - scrollY
	}
	return Viewport{X: anchor.X, Y: anchor.Y, W: anchor.W, H: anchor.H}, true
}
