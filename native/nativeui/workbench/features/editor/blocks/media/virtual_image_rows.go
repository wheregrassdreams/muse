package media

// VirtualImageRow is the source-backed Vim caret position inside a tall,
// standalone image. The source offset remains the image's Markdown line.
type VirtualImageRow struct {
	LineStart int
	Row       int
}

// VirtualImageRows owns the transient row position used to navigate the
// visual height of a standalone image without changing document source.
type VirtualImageRows struct {
	state *VirtualImageRow
}

func (v *VirtualImageRows) Clear() { v.state = nil }

func (v *VirtualImageRows) State() (VirtualImageRow, bool) {
	if v == nil || v.state == nil {
		return VirtualImageRow{}, false
	}
	return *v.state, true
}

func (v *VirtualImageRows) At(lineStart int) bool {
	state, ok := v.State()
	return ok && state.LineStart == lineStart
}

// Enter selects the first or last virtual row according to movement
// direction. Images shorter than two text rows do not need virtual movement.
func (v *VirtualImageRows) Enter(lineStart, rows, direction int) bool {
	if rows < 2 {
		v.Clear()
		return false
	}
	row := 0
	if direction < 0 {
		row = rows - 1
	}
	v.state = &VirtualImageRow{LineStart: lineStart, Row: row}
	return true
}

// Move updates the virtual row. It clears the position and returns false
// when movement crosses an image edge so ordinary document navigation can
// take over.
func (v *VirtualImageRows) Move(direction, rows int) bool {
	if v == nil || v.state == nil || direction == 0 || rows < 2 {
		return false
	}
	next := v.state.Row + direction
	if next < 0 || next >= rows {
		v.Clear()
		return false
	}
	v.state.Row = next
	return true
}

// CaretY returns the y offset of a virtual row, clamped to the rendered
// image bounds.
func (v *VirtualImageRows) CaretY(rows int, lineHeight float32) (row int, yOffset, height float32, ok bool) {
	state, exists := v.State()
	if !exists || rows < 2 || lineHeight <= 0 {
		return 0, 0, 0, false
	}
	row = min(max(state.Row, 0), rows-1)
	yOffset = float32(row) * lineHeight
	height = lineHeight
	return row, yOffset, height, true
}
