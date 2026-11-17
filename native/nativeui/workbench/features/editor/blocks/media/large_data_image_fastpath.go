package editor

import "muse/markdown/syntax"

// viewInlines uses the syntax-owned large-image scanner only for paragraph
// leaves. The parser remains the fallback for every other Markdown form.
func (e *Editor) viewInlines(l *leaf) []syntax.IncrementalInline {
	if l == nil || l.id < 0 || e.p.Parser() == nil {
		return nil
	}
	if imageOnly := e.p.ImageInlines(); len(imageOnly) > 0 && imageOnly[0].Start == l.start && imageOnly[0].End <= l.end {
		return imageOnly
	}
	if cached, ok := e.largeDataImageCache.Lookup(e.Session.Revision(), l.id); ok {
		return cached
	}
	if l.kind == kPara {
		if inlines, ok := syntax.LargeDataImageInlines(e.Source(), l.start, l.end); ok {
			e.largeDataImageCache.Store(e.Session.Revision(), l.id, inlines)
			return inlines
		}
	}
	return e.p.Parser().Inlines(l.id)
}
