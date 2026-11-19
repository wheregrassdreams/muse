package viewport

import "testing"

func TestTransformTransitions(t *testing.T) {
	v := NewTransform()
	v.ZoomBy(100)
	v.PanBy(2, -3)
	v.BeginDrag(5, 8)
	dx, dy, ok := v.DragTo(7, 4)
	if !ok || dx != 2 || dy != -4 || v.X != 4 || v.Y != -7 || v.Zoom != 8 {
		t.Fatalf("drag transform = %+v, delta=(%v,%v), active=%v", v, dx, dy, ok)
	}
	v.EndDrag()
	v.Reset()
	if v.Zoom != 1 || v.X != 0 || v.Y != 0 || v.Dragging || ClampZoom(0) != 0.1 {
		t.Fatalf("reset transform = %+v", v)
	}
}
