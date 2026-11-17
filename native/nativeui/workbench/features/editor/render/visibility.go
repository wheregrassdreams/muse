package render

import "sort"

// VerticalBounds is the laid-out vertical extent of a block in document
// coordinates.
type VerticalBounds struct {
	Y float32
	H float32
}

// VisibleRange returns the half-open index range of ordered vertical blocks
// that intersects [top, bottom]. The callback lets the host provide cached or
// virtualized positions without exposing its block model to render.
func VisibleRange(count int, boundsAt func(int) VerticalBounds, top, bottom float32) (start, end int) {
	if count <= 0 || boundsAt == nil || bottom < top {
		return 0, 0
	}
	start = sort.Search(count, func(i int) bool {
		b := boundsAt(i)
		return b.Y+b.H >= top
	})
	end = sort.Search(count, func(i int) bool {
		return boundsAt(i).Y > bottom
	})
	if start > end {
		return end, end
	}
	return start, end
}
