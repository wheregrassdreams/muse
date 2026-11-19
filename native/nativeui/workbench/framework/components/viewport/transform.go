// Package viewport contains backend-independent view transform state.
package viewport

import "math"

type Transform struct {
	Zoom, X, Y, DragX, DragY float32
	Dragging                 bool
}

func NewTransform() Transform { return Transform{Zoom: 1} }
func (t *Transform) ZoomBy(scale float32) {
	if scale > 0 {
		t.Zoom = ClampZoom(t.Zoom * scale)
	}
}
func (t *Transform) PanBy(dx, dy float32)   { t.X, t.Y = t.X+dx, t.Y+dy }
func (t *Transform) BeginDrag(x, y float32) { t.Dragging, t.DragX, t.DragY = true, x, y }
func (t *Transform) EndDrag()               { t.Dragging = false }
func (t *Transform) Reset()                 { t.Zoom, t.X, t.Y = 1, 0, 0 }

func (t *Transform) DragTo(x, y float32) (dx, dy float32, ok bool) {
	if !t.Dragging {
		return 0, 0, false
	}
	dx, dy = x-t.DragX, y-t.DragY
	t.PanBy(dx, dy)
	t.DragX, t.DragY = x, y
	return dx, dy, true
}

func ScrollZoom(delta float32) float32 { return float32(math.Pow(1.12, float64(-delta/60))) }
func ClampZoom(zoom float32) float32   { return min(float32(8), max(float32(0.1), zoom)) }
