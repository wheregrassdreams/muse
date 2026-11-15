// Package diagrams owns backend-neutral parsing and SVG-to-label projection
// for Mermaid diagrams rendered by the sketch engine.
package diagrams

import (
	"image/color"

	"tuios.dev/sketch"
)

// Theme carries palette tokens accepted by sketch. Zero colors preserve the
// renderer's defaults.
type Theme struct {
	Bg, Fg, Muted, Line, Surface, SurfaceAlt, Border, Accent color.RGBA
	Radius                                                   float64
}

// Options supplies optional page theme and text measurement policy.
type Options struct {
	Theme   *Theme
	Measure func(text string, size float64, bold bool) float64
}

// Label is one text run in a diagram's SVG coordinate space.
type Label struct {
	X, Y, Size float32
	Bold       bool
	Color      color.RGBA
	Anchor     int
	Text       string
}

// Result separates vector shapes from labels so a host can rasterize shapes
// and draw labels with its own text renderer.
type Result struct {
	Width, Height float32
	Shapes        string
	Labels        []Label
}

// Parse renders Mermaid source to a backend-neutral diagram result.
func Parse(source string, opts Options) (Result, bool) {
	out, err := sketch.Render(source, sketch.RenderOptions{
		Theme:    sketchTheme(opts.Theme),
		Measurer: sketchMeasurer(opts.Measure),
	})
	if err != nil {
		return Result{}, false
	}
	w, h := float32(out.Width), float32(out.Height)
	resolved := resolveSVGVars(out.Data)
	if w <= 0 || h <= 0 {
		w, h = svgSize(resolved)
	}
	if w <= 0 || h <= 0 {
		return Result{}, false
	}
	texts := svgTexts(resolved)
	labels := make([]Label, len(texts))
	for i, tx := range texts {
		labels[i] = Label{X: tx.x, Y: tx.y, Size: tx.size, Bold: tx.bold, Color: tx.color, Anchor: tx.anchor, Text: tx.text}
	}
	return Result{Width: w, Height: h, Shapes: svgShapes(resolved), Labels: labels}, true
}

func sketchTheme(t *Theme) *sketch.Theme {
	if t == nil {
		return nil
	}
	th := &sketch.Theme{
		Bg: t.Bg, Fg: t.Fg, Muted: t.Muted, Line: t.Line,
		Surface: t.Surface, SurfaceAlt: t.SurfaceAlt, Border: t.Border, Accent: t.Accent,
	}
	if t.Radius > 0 {
		th.Radius = t.Radius
	}
	return th
}

type measureFunc func(text string, size float64, bold bool) float64

func (f measureFunc) Width(text string, size float64, style int) float64 {
	return f(text, size, style == 1)
}

func sketchMeasurer(m func(text string, size float64, bold bool) float64) sketch.TextMeasurer {
	if m == nil {
		return nil
	}
	return measureFunc(m)
}
