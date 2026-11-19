package render

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// HTMLLine is one source line after the editor host has removed enclosing
// Markdown container prefixes. Offset is the byte offset of Text in source.
type HTMLLine struct {
	Offset int
	Text   string
}

// HTMLSegmentKind identifies one visible part of an HTML block projection.
type HTMLSegmentKind uint8

const (
	HTMLText HTMLSegmentKind = iota
	HTMLAtom
	HTMLImage
)

// HTMLSegment is a detached display instruction. Source positions are byte
// offsets in the host document; the renderer retains neither lines nor
// segments after projection.
type HTMLSegment struct {
	Kind             HTMLSegmentKind
	SourceStart      int
	SourceEnd        int
	Text             string
	Style            ui.Span
	ImageSource, Alt string
	ImageWidth       float32
	ImageHeight      float32
	ImageZoom        float32
}

type htmlOpenStyle struct {
	name string
	prev InlineStyle
}

// ProjectHTMLBlock maps block HTML into styled display segments. The host
// supplies prefix-free source lines and performs image loading and object
// sizing; this renderer owns HTML whitespace, tag, and visual style rules.
func ProjectHTMLBlock(lines []HTMLLine, base Style, fontSize float32) []HTMLSegment {
	var (
		style   InlineStyle
		open    []htmlOpenStyle
		skip    string
		comment bool
		pre     int
		space   = -1
	)
	var segments []HTMLSegment
	emitted, atBreak := false, false
	wantBreaks := 0
	breakAt := [2]int{0, 0}
	span := func() ui.Span { return ResolveInlineStyle(base, style, fontSize) }
	appendBreaks := func() {
		if emitted {
			for i := 0; i < wantBreaks; i++ {
				segments = append(segments, HTMLSegment{Kind: HTMLAtom, SourceStart: breakAt[0], SourceEnd: breakAt[1], Text: "\n", Style: span()})
			}
			atBreak = true
		}
		wantBreaks = 0
		space = -1
	}
	flush := func(at int) {
		if wantBreaks > 0 {
			appendBreaks()
		}
		if space >= 0 {
			if emitted && !atBreak {
				segments = append(segments, HTMLSegment{Kind: HTMLAtom, SourceStart: space, SourceEnd: at, Text: " ", Style: span()})
			}
			space = -1
		}
	}
	text := func(s, e int, value string) {
		flush(s)
		if e > s {
			segments = append(segments, HTMLSegment{Kind: HTMLText, SourceStart: s, SourceEnd: e, Text: value, Style: span()})
			emitted, atBreak = true, false
		}
	}
	words := func(line string, off, s, e int) {
		if skip != "" || e <= s {
			return
		}
		if pre > 0 {
			text(off+s, off+e, line[s:e])
			return
		}
		for i := s; i < e; {
			if htmlSpace(line[i]) {
				if space < 0 {
					space = off + i
				}
				i++
				continue
			}
			j := i
			for j < e && !htmlSpace(line[j]) {
				j++
			}
			text(off+i, off+j, line[i:j])
			i = j
		}
	}
	boundary := func(s, e int) {
		if emitted {
			if wantBreaks < 1 {
				wantBreaks = 1
			}
			breakAt = [2]int{s, e}
		}
	}
	handle := func(tag string, s, e int) {
		name, closing, self := htmlTagParts(tag)
		if name == "" {
			return
		}
		if skip != "" {
			if closing && name == skip {
				skip = ""
			}
			return
		}
		if closing {
			if htmlBlockName(name) {
				boundary(s, e)
			}
			if name == "pre" && pre > 0 {
				pre--
			}
			for k := len(open) - 1; k >= 0; k-- {
				if open[k].name == name {
					style = open[k].prev
					open = open[:k]
					break
				}
			}
			return
		}
		switch name {
		case "br":
			if emitted {
				wantBreaks = max(wantBreaks+1, 1)
				breakAt = [2]int{s, e}
			}
			return
		case "hr":
			boundary(s, e)
			return
		case "img":
			if src, alt, size, ok := HTMLImageTag(tag); ok {
				flush(s)
				segments = append(segments, HTMLSegment{Kind: HTMLImage, SourceStart: s, SourceEnd: e, Style: span(), ImageSource: src, Alt: alt, ImageWidth: size.Width, ImageHeight: size.Height, ImageZoom: size.Zoom})
				emitted, atBreak = true, false
			}
			return
		case "script", "style", "template":
			if !self {
				skip = name
			}
			return
		}
		if htmlBlockName(name) {
			boundary(s, e)
		}
		if !self && !htmlVoid(name) {
			open = append(open, htmlOpenStyle{name: name, prev: style})
		}
		if name == "pre" && !self {
			pre++
		}
		style = HTMLTagStyle(name, tag, style)
	}
	scan := func(line string, off int) {
		i := 0
		if comment {
			k := strings.Index(line, "-->")
			if k < 0 {
				return
			}
			comment = false
			i = k + 3
		}
		for i < len(line) {
			lt := strings.IndexByte(line[i:], '<')
			if lt < 0 {
				words(line, off, i, len(line))
				return
			}
			lt += i
			if lt > i {
				words(line, off, i, lt)
			}
			if strings.HasPrefix(line[lt:], "<!--") {
				if k := strings.Index(line[lt+4:], "-->"); k >= 0 {
					i = lt + 4 + k + 3
				} else {
					comment = true
					return
				}
				continue
			}
			gt := htmlTagEnd(line, lt+1)
			if gt < 0 {
				words(line, off, lt, len(line))
				return
			}
			handle(line[lt:gt+1], off+lt, off+gt+1)
			i = gt + 1
		}
	}
	for i, line := range lines {
		if i > 0 && skip == "" {
			if pre > 0 {
				if emitted {
					wantBreaks = max(wantBreaks+1, 1)
					breakAt = [2]int{line.Offset - 1, line.Offset}
				}
			} else if wantBreaks == 0 && space < 0 {
				space = line.Offset - 1
			}
		}
		scan(line.Text, line.Offset)
	}
	return segments
}

func htmlVoid(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

func htmlBlockName(name string) bool {
	switch name {
	case "address", "article", "aside", "blockquote", "dd", "details", "dialog", "div", "dl", "dt", "fieldset", "figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hgroup", "li", "main", "nav", "ol", "p", "pre", "section", "summary", "table", "tbody", "td", "tfoot", "th", "thead", "tr", "ul":
		return true
	}
	return false
}

func htmlTagParts(tag string) (name string, closing, self bool) {
	t := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(tag), "<"), ">"))
	if strings.HasPrefix(t, "/") {
		closing = true
		t = t[1:]
	}
	if strings.HasSuffix(t, "/") {
		self = true
		t = strings.TrimSuffix(t, "/")
	}
	t = strings.TrimSpace(t)
	i := 0
	for i < len(t) {
		c := t[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == ':') {
			break
		}
		i++
	}
	return strings.ToLower(t[:i]), closing, self
}

func htmlTagEnd(s string, i int) int {
	var quote byte
	for ; i < len(s); i++ {
		switch c := s[i]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return i
		}
	}
	return -1
}

func htmlSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }
