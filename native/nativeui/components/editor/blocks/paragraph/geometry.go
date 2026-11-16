package paragraphui

import "github.com/egoist/mygo/ui"

// ChevronBounds returns the rectangle used for a fold control adjacent to a
// heading's first laid out line.
func ChevronBounds(headingX, headingY, lineY, lineHeight, fontSize float32) ui.Rect {
	size := fontSize * 0.9
	return ui.Rect{
		X: headingX - fontSize*1.15,
		Y: headingY + lineY + (lineHeight-size)/2,
		W: size,
		H: size,
	}
}

// HeadingLineHit reports whether a pointer is in one laid out line of a
// heading. The first line extends through the fold-control corridor.
func HeadingLineHit(line ui.Rect, first bool, chevron ui.Rect, x, y float32) bool {
	if y < line.Y || y > line.Y+line.H {
		return false
	}
	left, right := line.X, line.X+line.W
	if first {
		left = min(left, chevron.X-chevron.W*0.2)
		right = max(right, chevron.X+chevron.W*1.2)
	}
	return x >= left && x <= right
}

// ChevronCorridorHit reports whether a pointer falls into the gap between a
// heading's first line and its icon.
func ChevronCorridorHit(chevron ui.Rect, x, y float32) bool {
	return y >= chevron.Y-chevron.H*0.1 && y <= chevron.Y+chevron.H*1.2 &&
		x >= chevron.X-chevron.W*0.2 && x <= chevron.X+chevron.W*1.2
}

// ChevronHit reports whether a pointer falls inside the expanded fold-control
// target used by the editor host.
func ChevronHit(chevron ui.Rect, x, y float32) bool {
	return x >= chevron.X-chevron.W*0.2 && x <= chevron.X+chevron.W*1.2 &&
		y >= chevron.Y-chevron.H*0.1 && y <= chevron.Y+chevron.H*1.2
}
