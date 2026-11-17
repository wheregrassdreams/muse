package media

import (
	"strconv"
	"strings"
)

// AltWidth extracts an optional terminal |width suffix from an image alt.
func AltWidth(alt string) (string, float32) {
	pipe := strings.LastIndexByte(alt, '|')
	if pipe < 0 {
		return alt, 0
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(alt[pipe+1:]), 32)
	if err != nil || n <= 0 {
		return alt, 0
	}
	return strings.TrimSpace(alt[:pipe]), float32(n)
}

// AltWidthRange returns the terminal width suffix bounds for a Markdown image
// source range. When there is no suffix, it returns the insertion point before
// the closing alt bracket.
func AltWidthRange(src string, from, to int) (int, int) {
	if from < 0 || to > len(src) || from >= to {
		return 0, 0
	}
	open := strings.Index(src[from:to], "![")
	if open < 0 {
		return 0, 0
	}
	start := from + open + 2
	depth, close := 1, -1
	for i := start; i < to; i++ {
		if src[i] == '\\' {
			i++
			continue
		}
		switch src[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				close = i
				i = to
			}
		}
	}
	if close < 0 {
		return 0, 0
	}
	if pipe := strings.LastIndexByte(src[start:close], '|'); pipe >= 0 {
		at := start + pipe
		if _, err := strconv.ParseFloat(strings.TrimSpace(src[at+1:close]), 32); err == nil {
			return at + 1, close
		}
	}
	return close, close
}

// WikiImageWidth reads the optional width from a wiki image's label range.
func WikiImageWidth(src string, labelFrom, labelTo int) ImageSize {
	if labelFrom < 0 || labelTo > len(src) || labelFrom >= labelTo {
		return ImageSize{}
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(src[labelFrom:labelTo]), 32)
	if err != nil || n <= 0 {
		return ImageSize{}
	}
	return ImageSize{Width: float32(n)}
}

// ResizeImageSource writes a source-level width edit through the editor host.
// Parsing and syntax preservation belong to media; transaction policy remains
// with the host.
func ResizeImageSource(host ImageResizeHost, image ImageSource, width, intrinsicWidth, intrinsicHeight float32) bool {
	if host == nil || width < 1 {
		return false
	}
	src := host.Source()
	if image.From < 0 || image.To > len(src) || image.From >= image.To {
		return false
	}
	whole := src[image.From:image.To]
	value := strconv.Itoa(int(width))
	var replacement string
	switch image.Syntax {
	case ImageSyntaxWiki:
		open := strings.Index(whole, "[[")
		close := strings.LastIndex(whole, "]]")
		if open < 0 || close <= open {
			return false
		}
		bodyStart := open + 2
		body := whole[bodyStart:close]
		if pipe := strings.LastIndexByte(body, '|'); pipe >= 0 {
			body = body[:pipe+1] + value
		} else {
			body += "|" + value
		}
		replacement = whole[:bodyStart] + body + whole[close:]
	case ImageSyntaxHTML:
		i := strings.Index(strings.ToLower(whole), "<img")
		if i < 0 {
			return false
		}
		end := htmlTagEnd(whole, i+4)
		if end < 0 {
			return false
		}
		tag := whole[i : end+1]
		attrs := []htmlAttribute{{name: "width", value: value}}
		if strings.Contains(strings.ToLower(tag), "height") && intrinsicWidth > 0 && intrinsicHeight > 0 {
			attrs = append(attrs, htmlAttribute{name: "height", value: strconv.Itoa(int(width * intrinsicHeight / intrinsicWidth))})
		}
		replacement = whole[:i] + setHTMLAttrs(tag, attrs...) + whole[i+len(tag):]
	case ImageSyntaxMarkdown:
		if image.WidthFrom < 0 || image.WidthTo < image.WidthFrom || image.WidthTo > len(src) {
			return false
		}
		lo, hi := image.WidthFrom-image.From, image.WidthTo-image.From
		if lo < 2 || hi >= len(whole) || whole[hi] != ']' {
			return false
		}
		suffix := whole[hi:]
		// Expand collapsed/shortcut references before inserting the width so
		// the alt remains the lookup key used by CommonMark.
		key := whole[2:hi]
		if suffix == "]" {
			suffix += "[" + key + "]"
		} else if strings.HasPrefix(suffix, "][]") {
			suffix = "][" + key + "]" + suffix[3:]
		}
		if lo == hi {
			value = "|" + value
		}
		replacement = whole[:lo] + value + suffix
	default:
		return false
	}
	if replacement == "" || replacement == whole {
		return false
	}
	host.CommitImageResize(image.From, image.To, replacement)
	return true
}

// ImageAlignmentState tracks host-local presentation overrides by source
// offset. Standalone images default to centered; false means explicitly left.
type ImageAlignmentState struct {
	centered map[int]bool
}

// ImageResizeGesture keeps the media resize interaction's width calculation
// independent of the editor's input event and hit-test representation.
type ImageResizeGesture struct {
	startX, startY, startWidth float32
	previewWidth               float32
}

func NewImageResizeGesture(x, y, width float32) ImageResizeGesture {
	return ImageResizeGesture{startX: x, startY: y, startWidth: width, previewWidth: width}
}

func (g *ImageResizeGesture) Drag(x, y float32) float32 {
	if g == nil {
		return 0
	}
	g.previewWidth = max(40, g.startWidth+(x-g.startX+y-g.startY)*.5)
	return g.previewWidth
}

func (g ImageResizeGesture) PreviewWidth() float32 { return g.previewWidth }

func (s *ImageAlignmentState) Centered(sourceOffset int) bool {
	if s == nil || s.centered == nil {
		return true
	}
	centered, exists := s.centered[sourceOffset]
	return !exists || centered
}

func (s *ImageAlignmentState) Toggle(sourceOffset int) bool {
	if s == nil || sourceOffset < 0 {
		return true
	}
	if s.centered == nil {
		s.centered = make(map[int]bool)
	}
	next := !s.Centered(sourceOffset)
	s.centered[sourceOffset] = next
	return next
}

func (s *ImageAlignmentState) Shift(from, to, inserted int) {
	if s == nil || len(s.centered) == 0 {
		return
	}
	shift := inserted - (to - from)
	next := make(map[int]bool, len(s.centered))
	for at, centered := range s.centered {
		switch {
		case at >= to:
			next[at+shift] = centered
		case at < from:
			next[at] = centered
		}
	}
	s.centered = next
}

func (s *ImageAlignmentState) Clear() {
	if s != nil {
		s.centered = nil
	}
}

type htmlAttribute struct{ name, value string }

func setHTMLAttrs(tag string, attrs ...htmlAttribute) string {
	insert := len(tag) - 1
	for insert > 0 && strings.ContainsRune("/ \t\r\n", rune(tag[insert-1])) {
		insert--
	}
	for _, a := range attrs {
		p := strings.IndexByte(tag, ' ')
		if p < 0 {
			p = 4
		}
		found := false
		for p < insert {
			for p < insert && strings.ContainsRune("/ \t\r\n", rune(tag[p])) {
				p++
			}
			start := p
			for p < insert && !strings.ContainsRune(" =\t\r\n/>", rune(tag[p])) {
				p++
			}
			if start == p {
				p++
				continue
			}
			name := strings.ToLower(tag[start:p])
			for p < insert && strings.ContainsRune(" \t\r\n", rune(tag[p])) {
				p++
			}
			if p >= insert || tag[p] != '=' {
				continue
			}
			p++
			for p < insert && strings.ContainsRune(" \t\r\n", rune(tag[p])) {
				p++
			}
			v0, v1 := p, p
			if p < insert && (tag[p] == '\'' || tag[p] == '"') {
				q := tag[p]
				v0 = p + 1
				v1 = v0
				for v1 < insert && tag[v1] != q {
					v1++
				}
				p = min(v1+1, insert)
			} else {
				for p < insert && !strings.ContainsRune(" \t\r\n/>", rune(tag[p])) {
					p++
				}
				v1 = p
			}
			if name == a.name {
				tag = tag[:v0] + a.value + tag[v1:]
				insert += len(a.value) - (v1 - v0)
				found = true
				break
			}
		}
		if !found {
			tag = tag[:insert] + ` ` + a.name + `="` + a.value + `"` + tag[insert:]
			insert += len(a.name) + len(a.value) + 4
		}
	}
	return tag
}

func htmlTagEnd(s string, i int) int {
	var quote byte
	for ; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '>':
			return i
		}
	}
	return -1
}
