package editor

import (
	"image/color"
	"strings"

	"github.com/egoist/mygo/ui"
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
		Bg: diagramColor(e.lk.panel), Fg: diagramColor(e.lk.text), Muted: diagramColor(e.lk.muted),
		Line: diagramColor(e.lk.border), Surface: diagramColor(e.lk.panel), SurfaceAlt: diagramColor(e.lk.codeBg),
		Border: diagramColor(e.lk.border), Accent: diagramColor(e.lk.accent), Radius: 4,
	}
}

func diagramColor(c ui.Color) color.RGBA {
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: c.A}
}
