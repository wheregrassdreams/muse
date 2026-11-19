package render

import (
	"html"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// InlineStyle is the editor-independent inline presentation state produced
// from Markdown and HTML semantics.
type InlineStyle struct {
	Bold, Italic, Strike, Code, Mono, Link, Tag, Under, Mark, Sup, Sub, Kbd bool
	LinkHover, LinkInvalid, CodeJoined, Marker, Muted, Heading              bool
	Color, Background                                                       ui.Color
}

// ImageSize is the DIP size and zoom requested by an HTML image tag.
type ImageSize struct{ Width, Height, Zoom float32 }

// HTMLAttributes reads lower-case attribute names and unescapes values.
func HTMLAttributes(tag string) map[string]string {
	out := map[string]string{}
	s := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(tag), "<"), ">")
	i := strings.IndexAny(s, " \t\r\n/")
	if i < 0 {
		return out
	}
	s = s[i:]
	for {
		s = strings.TrimLeft(s, " \t\r\n/")
		if s == "" {
			return out
		}
		j := strings.IndexAny(s, " \t\r\n=/")
		if j < 0 {
			j = len(s)
		}
		if j == 0 {
			s = s[1:]
			continue
		}
		name := strings.ToLower(s[:j])
		s = strings.TrimLeft(s[j:], " \t\r\n")
		if !strings.HasPrefix(s, "=") {
			out[name] = ""
			continue
		}
		s = strings.TrimLeft(s[1:], " \t\r\n")
		var value string
		if s != "" && (s[0] == '"' || s[0] == '\'') {
			if k := strings.IndexByte(s[1:], s[0]); k >= 0 {
				value, s = s[1:1+k], s[2+k:]
			} else {
				value, s = s[1:], ""
			}
		} else {
			k := strings.IndexAny(s, " \t\r\n")
			if k < 0 {
				k = len(s)
			}
			value, s = s[:k], s[k:]
		}
		out[name] = html.UnescapeString(value)
	}
}

// StyleProperty returns one CSS declaration value, ignoring !important.
func StyleProperty(style, name string) string {
	for _, declaration := range strings.Split(style, ";") {
		if key, value, ok := strings.Cut(declaration, ":"); ok && strings.EqualFold(strings.TrimSpace(key), name) {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "!important"))
		}
	}
	return ""
}

// HTMLTagStyle applies supported HTML formatting and CSS declarations to a
// detached inline style value.
func HTMLTagStyle(name, tag string, style InlineStyle) InlineStyle {
	switch name {
	case "b", "strong", "summary", "h1", "h2", "h3", "h4", "h5", "h6":
		style.Bold = true
	case "i", "em", "cite", "dfn", "var":
		style.Italic = true
	case "u", "ins":
		style.Under = true
	case "s", "del", "strike":
		style.Strike = true
	case "mark":
		style.Mark = true
	case "sup":
		style.Sup = true
	case "sub":
		style.Sub = true
	case "a":
		style.Link = true
	case "kbd":
		style.Kbd = true
	case "code":
		style.Code = true
	case "tt", "samp", "pre":
		style.Mono = true
	case "blockquote":
		style.Muted = true
	}
	attrs := HTMLAttributes(tag)
	css := attrs["style"]
	color := attrs["color"]
	if value := StyleProperty(css, "color"); value != "" {
		color = value
	}
	if value, ok := CSSColor(color); ok {
		style.Color = value
	}
	background := StyleProperty(css, "background-color")
	if background == "" {
		background = StyleProperty(css, "background")
	}
	if value, ok := CSSColor(background); ok {
		style.Background = value
	}
	switch weight := StyleProperty(css, "font-weight"); weight {
	case "bold", "bolder", "600", "700", "800", "900":
		style.Bold = true
	}
	if StyleProperty(css, "font-style") == "italic" {
		style.Italic = true
	}
	decoration := StyleProperty(css, "text-decoration")
	if strings.Contains(decoration, "underline") {
		style.Under = true
	}
	if strings.Contains(decoration, "line-through") {
		style.Strike = true
	}
	return style
}

// CSSColor parses supported CSS hexadecimal, RGB and named color values.
func CSSColor(value string) (ui.Color, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ui.Color{}, false
	}
	if h, ok := strings.CutPrefix(value, "#"); ok {
		switch len(h) {
		case 3, 4, 6, 8:
		default:
			return ui.Color{}, false
		}
		for _, c := range h {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return ui.Color{}, false
			}
		}
		return ui.Hex(h), true
	}
	if strings.HasPrefix(value, "rgb") {
		open, end := strings.IndexByte(value, '('), strings.LastIndexByte(value, ')')
		if open < 0 || end < open {
			return ui.Color{}, false
		}
		parts := strings.FieldsFunc(value[open+1:end], func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
		if len(parts) < 3 {
			return ui.Color{}, false
		}
		var channels [4]float64
		channels[3] = 1
		for i := 0; i < len(parts) && i < 4; i++ {
			part := parts[i]
			f, err := strconv.ParseFloat(strings.TrimSuffix(part, "%"), 64)
			if err != nil {
				return ui.Color{}, false
			}
			switch {
			case strings.HasSuffix(part, "%") && i < 3:
				f = f * 255 / 100
			case strings.HasSuffix(part, "%"):
				f /= 100
			}
			channels[i] = f
		}
		channel := func(f float64) uint8 { return uint8(min(max(f, 0), 255)) }
		return ui.RGBA(channel(channels[0]), channel(channels[1]), channel(channels[2]), float32(min(max(channels[3], 0), 1))), true
	}
	if hex, ok := namedColors[value]; ok {
		return ui.Hex(hex), true
	}
	return ui.Color{}, false
}

// HTMLImageTag reads an <img> source, alt text, and requested sizing.
func HTMLImageTag(tag string) (src, alt string, size ImageSize, ok bool) {
	t := strings.TrimSpace(tag)
	if len(t) < 5 || !strings.EqualFold(t[:4], "<img") || !strings.ContainsAny(t[4:5], " \t\r\n/>") {
		return "", "", size, false
	}
	attrs := HTMLAttributes(t)
	if attrs["src"] == "" {
		return "", "", size, false
	}
	style := attrs["style"]
	size.Width, size.Height = CSSLength(attrs["width"]), CSSLength(attrs["height"])
	if width := CSSLength(StyleProperty(style, "width")); width > 0 {
		size.Width = width
	}
	if height := CSSLength(StyleProperty(style, "height")); height > 0 {
		size.Height = height
	}
	if zoom := StyleProperty(style, "zoom"); zoom != "" {
		percent := strings.HasSuffix(zoom, "%")
		if factor, err := strconv.ParseFloat(strings.TrimSuffix(zoom, "%"), 32); err == nil && factor > 0 {
			if percent {
				factor /= 100
			}
			size.Zoom = float32(factor)
		}
	}
	return attrs["src"], attrs["alt"], size, true
}

// CSSLength reads a positive pixel dimension, with or without a px suffix.
func CSSLength(value string) float32 {
	value = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), "px"))
	factor, err := strconv.ParseFloat(value, 32)
	if err != nil || factor <= 0 {
		return 0
	}
	return float32(factor)
}

var namedColors = map[string]string{
	"black": "000000", "white": "ffffff", "gray": "808080", "grey": "808080",
	"silver": "c0c0c0", "red": "ff0000", "maroon": "800000", "orange": "ffa500",
	"yellow": "ffff00", "olive": "808000", "lime": "00ff00", "green": "008000",
	"aqua": "00ffff", "cyan": "00ffff", "teal": "008080", "blue": "0000ff",
	"navy": "000080", "fuchsia": "ff00ff", "magenta": "ff00ff", "purple": "800080",
	"pink": "ffc0cb", "hotpink": "ff69b4", "deeppink": "ff1493", "brown": "a52a2a",
	"gold": "ffd700", "violet": "ee82ee", "indigo": "4b0082", "crimson": "dc143c",
	"coral": "ff7f50", "tomato": "ff6347", "orangered": "ff4500", "salmon": "fa8072",
	"orchid": "da70d6", "plum": "dda0dd", "khaki": "f0e68c", "tan": "d2b48c",
	"chocolate": "d2691e", "firebrick": "b22222", "darkred": "8b0000",
	"darkorange": "ff8c00", "darkgreen": "006400", "forestgreen": "228b22",
	"seagreen": "2e8b57", "limegreen": "32cd32", "darkblue": "00008b",
	"royalblue": "4169e1", "steelblue": "4682b4", "dodgerblue": "1e90ff",
	"deepskyblue": "00bfff", "skyblue": "87ceeb", "lightblue": "add8e6",
	"lightgreen": "90ee90", "lightyellow": "ffffe0", "lightgray": "d3d3d3",
	"lightgrey": "d3d3d3", "darkgray": "a9a9a9", "darkgrey": "a9a9a9",
	"slategray": "708090", "turquoise": "40e0d0", "darkviolet": "9400d3",
}
