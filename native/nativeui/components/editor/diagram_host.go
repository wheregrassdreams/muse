package editor

import (
	"image/color"
	"strings"

	"muse/markdown/diagrams"
)

// previewDiagram exports a detached diagram model to the app-owned preview
// surface. It only reads the source and current theme; it does not change the
// source, selection, caret, or inline diagram state.
func (e *Editor) previewDiagram(source string) bool {
	if e.opts.OnDiagramPreview == nil {
		return false
	}
	result, ok := diagrams.Parse(source, diagrams.Options{Theme: e.diagramTheme()})
	if !ok {
		return false
	}
	result.Labels = append([]diagrams.Label(nil), result.Labels...)
	e.opts.OnDiagramPreview(strings.Clone(source), result)
	return true
}

func (e *Editor) diagramTheme() *diagrams.Theme {
	if e.lk == (look{}) {
		return nil
	}
	return &diagrams.Theme{
		Bg:         color.RGBA{R: e.lk.panel.R, G: e.lk.panel.G, B: e.lk.panel.B, A: e.lk.panel.A},
		Fg:         color.RGBA{R: e.lk.text.R, G: e.lk.text.G, B: e.lk.text.B, A: e.lk.text.A},
		Muted:      color.RGBA{R: e.lk.muted.R, G: e.lk.muted.G, B: e.lk.muted.B, A: e.lk.muted.A},
		Line:       color.RGBA{R: e.lk.border.R, G: e.lk.border.G, B: e.lk.border.B, A: e.lk.border.A},
		Surface:    color.RGBA{R: e.lk.panel.R, G: e.lk.panel.G, B: e.lk.panel.B, A: e.lk.panel.A},
		SurfaceAlt: color.RGBA{R: e.lk.codeBg.R, G: e.lk.codeBg.G, B: e.lk.codeBg.B, A: e.lk.codeBg.A},
		Border:     color.RGBA{R: e.lk.border.R, G: e.lk.border.G, B: e.lk.border.B, A: e.lk.border.A},
		Accent:     color.RGBA{R: e.lk.accent.R, G: e.lk.accent.G, B: e.lk.accent.B, A: e.lk.accent.A}, Radius: 4,
	}
}
