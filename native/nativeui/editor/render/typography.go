package render

// HeadingStyle is a neutral typography recipe. The host can use it for any
// hierarchical block without importing a Markdown package.
type HeadingStyle struct {
	Size       float32
	Weight     int
	LineHeight float32
	TopMargin  float32
}

func StyleHeading(level int, body float32) HeadingStyle {
	level = min(max(level, 1), 6)
	scale := [...]float32{2.5, 1.7, 1.35, 1.15, 1.05, 1}
	space := [...]float32{3.2, 3.2, 2.5, 2.2, 2, 2}
	weight, lineHeight := 600, float32(1.35)
	if level <= 2 {
		weight, lineHeight = 700, 1.25
	}
	return HeadingStyle{Size: body * scale[level-1], Weight: weight, LineHeight: lineHeight, TopMargin: body * space[level-1]}
}

func HeadingSize(level int, body float32) float32 { return StyleHeading(level, body).Size }

func HeadingWeight(level int) int { return StyleHeading(level, 16).Weight }

func HeadingLineHeight(level int) float32 { return StyleHeading(level, 16).LineHeight }

func HeadingTopMargin(level int, body float32) float32 {
	return StyleHeading(level, body).TopMargin
}
