package searchui

import (
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
)

func searchText(c *ui.Context, text, query string, color ui.Color, size float32) ui.Element {
	runes := []rune(text)
	lower := make([]rune, len(runes))
	for i, r := range runes {
		lower[i] = unicode.ToLower(r)
	}
	hits := make(map[int]struct{})
	for _, term := range strings.Fields(query) {
		needle := []rune(term)
		for i := range needle {
			needle[i] = unicode.ToLower(needle[i])
		}
		if len(needle) == 0 || len(needle) > len(lower) {
			continue
		}
		for start := 0; start+len(needle) <= len(lower); {
			matched := true
			for j, r := range needle {
				if lower[start+j] != r {
					matched = false
					break
				}
			}
			if !matched {
				start++
				continue
			}
			for j := range needle {
				hits[start+j] = struct{}{}
			}
			start += len(needle)
		}
	}
	if len(hits) == 0 {
		return ui.RichText(c, ui.Span{Text: text}).FontSize(size).TextColor(color).SingleLine().Ellipsis("…")
	}
	spans := make([]ui.Span, 0, len(hits)*2+1)
	theme := c.Theme()
	for start := 0; start < len(runes); {
		_, highlighted := hits[start]
		end := start + 1
		for end < len(runes) {
			_, nextHighlighted := hits[end]
			if nextHighlighted != highlighted {
				break
			}
			end++
		}
		span := ui.Span{Text: string(runes[start:end])}
		if highlighted {
			span.Color = theme.Accent
			span.Background = ui.RGBA(theme.Accent.R, theme.Accent.G, theme.Accent.B, 0.18)
			span.Radius = 2
		}
		spans = append(spans, span)
		start = end
	}
	return ui.RichText(c, spans...).FontSize(size).TextColor(color).SingleLine().Ellipsis("…")
}
