package editor

import (
	"strings"

	"github.com/wheregrassdreams/muse-editor/markdown/diagrams"
	media "muse/nativeui/editor/blocks/media"
)

// previewDiagram exports a detached diagram model to the app-owned preview
// surface. It only reads the source and current theme; it does not change the
// source, selection, caret, or inline diagram state.
func (e *Editor) previewDiagram(source string) bool {
	if e.opts.OnDiagramPreview == nil {
		return false
	}
	result, ok := media.PreviewDiagram(source, e.diagramTheme())
	if !ok {
		return false
	}
	e.opts.OnDiagramPreview(strings.Clone(source), result)
	return true
}

func (e *Editor) diagramTheme() *diagrams.Theme {
	if e.lk == (look{}) {
		return nil
	}
	return media.DiagramTheme(e.lk.panel, e.lk.text, e.lk.muted, e.lk.border, e.lk.codeBg, e.lk.accent)
}
