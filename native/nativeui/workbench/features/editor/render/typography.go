package render

import markdownrender "muse/markdown/render"

// HeadingStyle resolves the shared Markdown heading metrics for this editor
// frame. It delegates the cross-product typography scale to the neutral
// Markdown renderer, keeping host widgets out of that policy.
func HeadingStyle(level int, body float32) markdownrender.HeadingStyle {
	return markdownrender.StyleHeading(level, body)
}

func HeadingSize(level int, body float32) float32 { return HeadingStyle(level, body).Size }

func HeadingWeight(level int) int { return HeadingStyle(level, 16).Weight }

func HeadingLineHeight(level int) float32 { return HeadingStyle(level, 16).LineHeight }

func HeadingTopMargin(level int, body float32) float32 {
	return HeadingStyle(level, body).TopMargin
}
