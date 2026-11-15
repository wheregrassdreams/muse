package render

// HeadingStyle is the backend-neutral visual scale for a Markdown heading.
type HeadingStyle struct {
	Size       float32
	Weight     int
	LineHeight float32
	TopMargin  float32
}

// StyleHeading resolves the document's heading scale at a body font size.
func StyleHeading(level int, body float32) HeadingStyle {
	level = clampLevel(level)
	scale := [...]float32{2.5, 1.7, 1.35, 1.15, 1.05, 1}
	space := [...]float32{3.2, 3.2, 2.5, 2.2, 2, 2}
	weight, lineHeight := 600, float32(1.35)
	if level <= 2 {
		weight, lineHeight = 700, 1.25
	}
	return HeadingStyle{Size: body * scale[level-1], Weight: weight, LineHeight: lineHeight, TopMargin: body * space[level-1]}
}

func clampLevel(level int) int { return min(max(level, 1), 6) }
