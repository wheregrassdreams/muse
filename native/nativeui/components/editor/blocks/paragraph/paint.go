package paragraphui

import "github.com/egoist/mygo/ui"

// PaintChevron draws a fold affordance into a detached rectangle. The host
// supplies folded state and palette values; paragraph UI owns the path shape.
func PaintChevron(p *ui.Painter, bounds ui.Rect, folded bool, stroke ui.Color, strokeWidth float32) {
	path := ui.Path{}
	if folded {
		path.MoveTo(bounds.X+bounds.W*0.38, bounds.Y+bounds.H*0.22)
		path.LineTo(bounds.X+bounds.W*0.68, bounds.Y+bounds.H*0.5)
		path.LineTo(bounds.X+bounds.W*0.38, bounds.Y+bounds.H*0.78)
	} else {
		path.MoveTo(bounds.X+bounds.W*0.22, bounds.Y+bounds.H*0.38)
		path.LineTo(bounds.X+bounds.W*0.5, bounds.Y+bounds.H*0.68)
		path.LineTo(bounds.X+bounds.W*0.78, bounds.Y+bounds.H*0.38)
	}
	p.StrokePath(&path, strokeWidth, stroke)
}
