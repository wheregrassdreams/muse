package editor

import (
	media "muse/nativeui/workbench/features/editor/blocks/media"
	"muse/nativeui/workbench/features/editor/render"
)

// htmlBlockDisplay adapts the render package's detached HTML projection into
// editor-owned pieces and image objects. Source and object lifetimes remain
// owned by the editor host.
func htmlBlockDisplay(b *builder, l *leaf) {
	lines := l.codeLines()
	input := make([]render.HTMLLine, 0, len(lines))
	for _, line := range lines {
		input = append(input, render.HTMLLine{Offset: line.off, Text: line.text})
	}
	for _, segment := range render.ProjectHTMLBlock(input, b.e.lk.renderStyle(), b.fontSize) {
		switch segment.Kind {
		case render.HTMLText:
			b.spanRun(segment.SourceStart, segment.SourceEnd, segment.Style)
		case render.HTMLAtom:
			b.spanAtom(segment.SourceStart, segment.SourceEnd, segment.Text, segment.Style)
		case render.HTMLImage:
			obj := b.e.sizedImage(segment.ImageSource, segment.Alt, media.ImageSize{
				Width: segment.ImageWidth, Height: segment.ImageHeight, Zoom: segment.ImageZoom,
			}, b.width, false)
			if obj != nil {
				obj.imageSource = media.ImageSource{From: segment.SourceStart, To: segment.SourceEnd, Syntax: media.ImageSyntaxHTML}
				obj.width = obj.w
				b.e.applyImagePreview(obj, segment.SourceStart, segment.SourceEnd)
			}
			b.atomSpan(segment.SourceStart, segment.SourceEnd, "", segment.Style, obj)
		}
	}
}
