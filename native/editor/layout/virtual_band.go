package layout

// ViewportBand is the vertical source-layout range around a viewport. The
// bounds are intentionally not clipped; the host owns document-boundary
// clipping and converting the band to visible blocks.
type ViewportBand struct {
	Top    float32
	Bottom float32
}

// VirtualBand computes the layout band for one viewport position.
//
// Ordinary movement retains half a viewport of overscan on both sides. When
// a previous laid-out position exists and the scroll jump is greater than
// half a viewport, the band uses one eighth of a viewport instead. The jump
// threshold is strict, matching the editor host's existing rule.
func VirtualBand(scrollY, viewportHeight, previousScrollY float32, hasPrevious bool) ViewportBand {
	buffer := viewportHeight * 0.5
	if hasPrevious && abs32(scrollY-previousScrollY) > viewportHeight*0.5 {
		buffer = viewportHeight * 0.125
	}
	return ViewportBand{
		Top:    scrollY - buffer,
		Bottom: scrollY + viewportHeight + buffer,
	}
}

func abs32(value float32) float32 {
	if value < 0 {
		return -value
	}
	return value
}
