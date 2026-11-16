package media

import (
	"image/color"

	"github.com/egoist/mygo/ui"
	"muse/markdown/diagrams"
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
