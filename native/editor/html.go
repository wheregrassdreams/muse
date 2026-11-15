package mdedit

import (
	"html"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// HTML within Markdown that Typora writes and draws: <span> and <font>
// coloring text, and <img> sized by its attributes or by a zoom, as
// Typora scales an image (style="zoom:50%").

// htmlAttrs returns the attributes of an HTML tag, by their names in lower
// case, their values unescaped.
func htmlAttrs(tag string) map[string]string {
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
		var v string
		if s != "" && (s[0] == '"' || s[0] == '\'') {
			if k := strings.IndexByte(s[1:], s[0]); k >= 0 {
				v, s = s[1:1+k], s[2+k:]
			} else {
				v, s = s[1:], ""
			}
		} else {
			k := strings.IndexAny(s, " \t\r\n")
			if k < 0 {
				k = len(s)
			}
			v, s = s[:k], s[k:]
		}
		out[name] = html.UnescapeString(v)
	}
}

// styleProp returns the value of property name in an inline style, as
// "color" in "color: red; font-weight: bold".
func styleProp(style, name string) string {
	for _, d := range strings.Split(style, ";") {
		if k, v, ok := strings.Cut(d, ":"); ok && strings.EqualFold(strings.TrimSpace(k), name) {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "!important"))
		}
	}
	return ""
}

// htmlStyle returns st with the formatting of an opening <span> or <font>
// tag: its color, background, weight, slant and decoration.
func htmlStyle(tag string, st istyle) istyle {
	a := htmlAttrs(tag)
	style := a["style"]
	color := a["color"]
	if v := styleProp(style, "color"); v != "" {
		color = v
	}
	if c, ok := cssColor(color); ok {
		st.color = c
	}
	bg := styleProp(style, "background-color")
	if bg == "" {
		bg = styleProp(style, "background")
	}
	if c, ok := cssColor(bg); ok {
		st.bg = c
	}
	switch w := styleProp(style, "font-weight"); w {
	case "bold", "bolder", "600", "700", "800", "900":
		st.bold = true
	}
	if styleProp(style, "font-style") == "italic" {
		st.italic = true
	}
	deco := styleProp(style, "text-decoration")
	if strings.Contains(deco, "underline") {
		st.under = true
	}
	if strings.Contains(deco, "line-through") {
		st.strike = true
	}
	return st
}

// htmlTagStyle returns st as the opening tag tag styles the text it
// holds: the elements HTML gives a look, and the color, background and
// decoration its attributes or style set.
func htmlTagStyle(name, tag string, st istyle) istyle {
	switch name {
	case "b", "strong", "summary", "h1", "h2", "h3", "h4", "h5", "h6":
		st.bold = true
	case "i", "em", "cite", "dfn", "var":
		st.italic = true
	case "u", "ins":
		st.under = true
	case "s", "del", "strike":
		st.strike = true
	case "mark":
		st.mark = true
	case "sup":
		st.sup = true
	case "sub":
		st.sub = true
	case "a":
		st.link = true
	case "kbd":
		st.kbd = true
	case "code":
		st.code = true
	case "tt", "samp", "pre":
		st.mono = true
	case "blockquote":
		st.muted = true
	}
	return htmlStyle(tag, st)
}

// cssColor reads a CSS color: #rgb, #rrggbb and their forms with alpha,
// rgb() and rgba(), and the common names.
func cssColor(v string) (ui.Color, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return ui.Color{}, false
	}
	if h, ok := strings.CutPrefix(v, "#"); ok {
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
	if strings.HasPrefix(v, "rgb") {
		open, end := strings.IndexByte(v, '('), strings.LastIndexByte(v, ')')
		if open < 0 || end < open {
			return ui.Color{}, false
		}
		parts := strings.FieldsFunc(v[open+1:end], func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
		if len(parts) < 3 {
			return ui.Color{}, false
		}
		var c [4]float64
		c[3] = 1
		for k := 0; k < len(parts) && k < 4; k++ {
			p := parts[k]
			f, err := strconv.ParseFloat(strings.TrimSuffix(p, "%"), 64)
			if err != nil {
				return ui.Color{}, false
			}
			switch {
			case strings.HasSuffix(p, "%") && k < 3:
				f = f * 255 / 100
			case strings.HasSuffix(p, "%"):
				f /= 100
			}
			c[k] = f
		}
		ch := func(f float64) uint8 { return uint8(min(max(f, 0), 255)) }
		return ui.RGBA(ch(c[0]), ch(c[1]), ch(c[2]), float32(min(max(c[3], 0), 1))), true
	}
	if h, ok := namedColors[v]; ok {
		return ui.Hex(h), true
	}
	return ui.Color{}, false
}

// namedColors are the CSS colors people name in documents.
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

// imgSize is the size an <img> asks for: width and height in DIPs, or 0
// for the image's own, and a zoom, or 0 for none.
type imgSize struct{ w, h, zoom float32 }

// imgTag reads an <img> tag: its source, its alt text and the size it
// asks for.
func imgTag(tag string) (src, alt string, sz imgSize, ok bool) {
	t := strings.TrimSpace(tag)
	if len(t) < 5 || !strings.EqualFold(t[:4], "<img") || !strings.ContainsAny(t[4:5], " \t\r\n/>") {
		return "", "", sz, false
	}
	a := htmlAttrs(t)
	if a["src"] == "" {
		return "", "", sz, false
	}
	style := a["style"]
	sz.w, sz.h = cssLength(a["width"]), cssLength(a["height"])
	if w := cssLength(styleProp(style, "width")); w > 0 {
		sz.w = w
	}
	if h := cssLength(styleProp(style, "height")); h > 0 {
		sz.h = h
	}
	if z := styleProp(style, "zoom"); z != "" {
		pct := strings.HasSuffix(z, "%")
		if f, err := strconv.ParseFloat(strings.TrimSuffix(z, "%"), 32); err == nil && f > 0 {
			if pct {
				f /= 100
			}
			sz.zoom = float32(f)
		}
	}
	return a["src"], a["alt"], sz, true
}

// cssLength reads a width or height in pixels, as 128 or "128px"; 0 for
// anything else.
func cssLength(v string) float32 {
	v = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), "px"))
	f, err := strconv.ParseFloat(v, 32)
	if err != nil || f <= 0 {
		return 0
	}
	return float32(f)
}
