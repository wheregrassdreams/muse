package editor

import (
	"strings"

	media "muse/nativeui/components/editor/blocks/media"
)

// Block-level HTML, as Typora writes it: <details> and <div> holding text,
// headings and images. The editor reads it far enough to draw what it
// holds: the tags are hidden, the text shows in the styles they give it, a
// block tag starts a line, <br> breaks one, and <img> draws its image. The
// block's source shows while the caret is in it, as a formula's TeX does,
// so the markup is edited as it is.

// htmlVoid reports whether a tag never has a closing tag.
func htmlVoid(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

// htmlBlockName reports whether a tag lays out as a block, on lines of its
// own.
func htmlBlockName(name string) bool {
	switch name {
	case "address", "article", "aside", "blockquote", "dd", "details",
		"dialog", "div", "dl", "dt", "fieldset", "figcaption", "figure",
		"footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header",
		"hgroup", "li", "main", "nav", "ol", "p", "pre", "section",
		"summary", "table", "tbody", "td", "tfoot", "th", "thead", "tr",
		"ul":
		return true
	}
	return false
}

// htmlTagParts reads a tag's name, lower cased, and whether it closes and
// closes itself, from the tag's text from < to >.
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

// htmlTagEnd returns the offset of the > that ends the tag from after its
// <, or -1: a > within a quoted attribute does not end it.
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

// htmlBlockDisplay adds the display of an HTML block leaf to b: its text,
// its images and its line breaks, its tags hidden.
func htmlBlockDisplay(b *builder, l *leaf) {
	var (
		st      istyle
		open    []htmlTag
		skip    string // script, style or template being skipped
		comment bool   // a <!-- that a line ended in
		pre     int    // how deep in <pre>
		space   = -1   // where the whitespace waiting for the next word starts
	)
	emitted, atBreak := false, false
	wantBreaks := 0
	breakAt := [2]int{0, 0}

	// boundary asks for a line of its own for the block at s..e.
	boundary := func(s, e int) {
		if !emitted {
			return
		}
		if wantBreaks < 1 {
			wantBreaks = 1
		}
		breakAt = [2]int{s, e}
	}
	// flush stands for the break and the space waiting, at the next word
	// at at.
	flush := func(at int) {
		if wantBreaks > 0 {
			if emitted {
				for k := 0; k < wantBreaks; k++ {
					b.atom(breakAt[0], breakAt[1], "\n", st, nil)
				}
				atBreak = true
			}
			wantBreaks = 0
			space = -1
		}
		if space >= 0 {
			if emitted && !atBreak {
				b.atom(space, at, " ", st, nil)
			}
			space = -1
		}
	}
	// text adds source text as it is.
	text := func(s, e int) {
		flush(s)
		b.spanRun(s, e, b.span(st))
		emitted, atBreak = true, false
	}
	// words adds text with its whitespace collapsed to spaces, as HTML
	// reads it, unless <pre> keeps it.
	words := func(s, e int) {
		if skip != "" {
			return
		}
		if pre > 0 {
			text(s, e)
			return
		}
		for i := s; i < e; {
			if htmlSpace(b.src[i]) {
				if space < 0 {
					space = i
				}
				i++
				continue
			}
			j := i
			for j < e && !htmlSpace(b.src[j]) {
				j++
			}
			text(i, j)
			i = j
		}
	}

	var handle func(tag string, s, e int)
	handle = func(tag string, s, e int) {
		name, closing, self := htmlTagParts(tag)
		if name == "" {
			return // <!doctype>, <?…> and the rest of the declarations
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
					st = open[k].prev
					open = open[:k]
					break
				}
			}
			return
		}
		switch name {
		case "br":
			// A line break within the text; one after another keeps the
			// empty line between them.
			if emitted {
				wantBreaks = max(wantBreaks+1, 1)
				breakAt = [2]int{s, e}
			}
			return
		case "hr":
			boundary(s, e)
			return
		case "img":
			if src, alt, sz, ok := imgTag(tag); ok {
				flush(s)
				obj := b.e.sizedImage(src, alt, media.ImageSize{Width: sz.w, Height: sz.h, Zoom: sz.zoom}, b.width, false)
				if obj != nil {
					obj.imageSource = media.ImageSource{From: s, To: e, Syntax: media.ImageSyntaxHTML}
					obj.width = obj.w
					b.e.applyImagePreview(obj, s, e)
				}
				b.atom(s, e, "", st, obj)
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
			open = append(open, htmlTag{name: name, prev: st})
		}
		if name == "pre" && !self {
			pre++
		}
		st = htmlTagStyle(name, tag, st)
	}

	scan := func(line string, off int) {
		i := 0
		if comment {
			// The rest of the comment the line before left open.
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
				words(off+i, off+len(line))
				return
			}
			lt += i
			if lt > i {
				words(off+i, off+lt)
			}
			if strings.HasPrefix(line[lt:], "<!--") {
				// A comment is hidden with what it holds.
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
				words(off+lt, off+len(line))
				return
			}
			handle(line[lt:gt+1], off+lt, off+gt+1)
			i = gt + 1
		}
	}

	for k, ln := range l.codeLines() {
		if k > 0 && skip == "" {
			// The line break between lines is whitespace, unless <pre>
			// keeps it.
			if pre > 0 {
				if emitted {
					wantBreaks = max(wantBreaks+1, 1)
					breakAt = [2]int{ln.off - 1, ln.off}
				}
			} else if wantBreaks == 0 && space < 0 {
				space = ln.off - 1
			}
		}
		scan(ln.text, ln.off)
	}
}

// htmlSpace reports whether c is whitespace to HTML.
func htmlSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}
