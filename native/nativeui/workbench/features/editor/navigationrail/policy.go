package navigationrail

import "strings"

// ShouldShow reports whether the rail has enough content and room to be
// useful. Heights and dimensions use the same logical coordinates as Frame.
func ShouldShow(itemCount int, documentHeight, scrollViewportHeight, viewportWidth, viewportHeight float32) bool {
	return itemCount > 1 && documentHeight > scrollViewportHeight+1 && viewportWidth > 52 && viewportHeight > 52
}

// Excerpt normalizes a section preview to one line and caps it at 140 runes.
func Excerpt(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if runes := []rune(text); len(runes) > 140 {
		return strings.TrimSpace(string(runes[:140])) + "…"
	}
	return text
}
