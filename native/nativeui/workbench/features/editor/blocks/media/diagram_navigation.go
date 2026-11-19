package media

// DiagramBarPressOffset positions the document offset under a pointer press
// on a horizontal scroll track. Presses on the thumb keep the current offset.
func DiagramBarPressOffset(offset, pointerX, trackX, trackWidth, thumbX, thumbWidth, maxOffset float32, onThumb bool) float32 {
	if onThumb {
		return clampDiagramOffset(offset, maxOffset)
	}
	travel := trackWidth - thumbWidth
	if travel <= 0 {
		return 0
	}
	position := clampDiagramOffset((pointerX-trackX-thumbWidth/2)/travel, 1)
	return position * maxOffset
}

// DiagramBarDragOffset maps a thumb drag to the corresponding content offset.
func DiagramBarDragOffset(startOffset, startX, pointerX, maxOffset, trackWidth, thumbWidth float32) float32 {
	travel := trackWidth - thumbWidth
	if travel <= 0 || maxOffset <= 0 {
		return 0
	}
	return clampDiagramOffset(startOffset+(pointerX-startX)*maxOffset/travel, maxOffset)
}

// DiagramPanOffset maps a body drag to a horizontal content offset.
func DiagramPanOffset(startOffset, startX, pointerX, maxOffset float32) float32 {
	return clampDiagramOffset(startOffset-(pointerX-startX), maxOffset)
}

// DiagramScrollOffset applies a wheel or trackpad delta to a horizontal
// content offset.
func DiagramScrollOffset(offset, delta, maxOffset float32) float32 {
	return clampDiagramOffset(offset+delta, maxOffset)
}

func clampDiagramOffset(offset, maxOffset float32) float32 {
	if maxOffset <= 0 {
		return 0
	}
	return min(max(offset, 0), maxOffset)
}
