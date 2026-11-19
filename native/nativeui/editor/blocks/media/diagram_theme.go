package media

import (
	"image/color"

	"github.com/egoist/mygo/ui"
	"github.com/wheregrassdreams/muse-editor/markdown/diagrams"
)

// DiagramTheme projects resolved editor colors into the renderer-neutral
// diagram theme model.
func DiagramTheme(panel, text, muted, border, surfaceAlt, accent ui.Color) *diagrams.Theme {
	colorOf := func(value ui.Color) color.RGBA {
		return color.RGBA{R: value.R, G: value.G, B: value.B, A: value.A}
	}
	return &diagrams.Theme{
		Bg: colorOf(panel), Fg: colorOf(text), Muted: colorOf(muted), Line: colorOf(border),
		Surface: colorOf(panel), SurfaceAlt: colorOf(surfaceAlt), Border: colorOf(border),
		Accent: colorOf(accent), Radius: 4,
	}
}

// PreviewDiagram parses a detached preview model and copies its mutable label
// slice before handing the result to another UI surface.
func PreviewDiagram(source string, theme *diagrams.Theme) (diagrams.Result, bool) {
	result, ok := diagrams.Parse(source, diagrams.Options{Theme: theme})
	if !ok {
		return diagrams.Result{}, false
	}
	result.Labels = append([]diagrams.Label(nil), result.Labels...)
	return result, true
}
