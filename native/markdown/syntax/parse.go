package syntax

import (
	"strconv"
	"strings"
)

// Parse parses Markdown source into a document. It never fails: anything
// the parser cannot place is kept as a paragraph so no text is lost, which
// is what makes the same code path usable for plain .txt files.
func Parse(src string) *Doc {
	return parseIncremental(src)
}

// ParseFile parses Markdown bytes, as read from disk.
func ParseFile(b []byte) *Doc { return Parse(string(b)) }

// htmlOnlyImage recognises an HTML block that holds nothing but one image, as
// READMEs often do with <p align="center"><img ...></p>, and turns it into a
// native image block. Text around the image makes it a normal HTML block.
func htmlOnlyImage(s string) (src, alt, title string, w, h float32, ok bool) {
	low := strings.ToLower(s)
	start := strings.Index(low, "<img")
	if start < 0 {
		return "", "", "", 0, 0, false
	}
	end := strings.Index(s[start:], ">")
	if end < 0 {
		return "", "", "", 0, 0, false
	}
	tag := s[start : start+end]
	src = HTMLAttribute(tag, "src")
	if src == "" {
		return "", "", "", 0, 0, false
	}
	w = cssSize(HTMLAttribute(tag, "width"))
	h = cssSize(HTMLAttribute(tag, "height"))
	// Everything outside the tag must be whitespace or other tags.
	rest := s[:start] + s[start+end+1:]
	for i := 0; i < len(rest); {
		if rest[i] == '<' {
			j := strings.IndexByte(rest[i:], '>')
			if j < 0 {
				return "", "", "", 0, 0, false
			}
			i += j + 1
			continue
		}
		if !strings.ContainsRune(" \t\r\n", rune(rest[i])) {
			return "", "", "", 0, 0, false
		}
		i++
	}
	return src, HTMLAttribute(tag, "alt"), HTMLAttribute(tag, "title"), w, h, true
}

// HTMLOnlyImage reports whether an HTML fragment contains only an image and
// surrounding tags/whitespace.
func HTMLOnlyImage(s string) (src, alt, title string, width, height float32, ok bool) {
	return htmlOnlyImage(s)
}

// cssSize reads an HTML width/height such as 128, "128px" or "12.5em".
func cssSize(v string) float32 {
	v = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(v), "px"))
	if v == "" {
		return 0
	}
	n, err := strconv.ParseFloat(v, 32)
	if err != nil || n < 0 {
		return 0
	}
	return float32(n)
}

// htmlOnlyHeading recognises a block that is just one h1–h6 tag, so an HTML
// README title keeps the heading scale instead of falling back to body text.
func htmlOnlyHeading(s string) (level int, text string, ok bool) {
	low := strings.ToLower(s)
	for l := 1; l <= 6; l++ {
		open := "<h" + strconv.Itoa(l)
		i := strings.Index(low, open)
		if i < 0 {
			continue
		}
		j := i + len(open)
		if j < len(s) && s[j] != '>' && s[j] != ' ' && s[j] != '\t' && s[j] != '\n' {
			continue
		}
		openEnd := strings.IndexByte(s[i:], '>')
		if openEnd < 0 {
			continue
		}
		bodyStart := i + openEnd + 1
		closeTag := "</h" + strconv.Itoa(l) + ">"
		closeAt := strings.Index(strings.ToLower(s[bodyStart:]), closeTag)
		if closeAt < 0 {
			continue
		}
		inner := s[bodyStart : bodyStart+closeAt]
		outside := s[:i] + s[bodyStart+closeAt+len(closeTag):]
		if strings.TrimSpace(HTMLText(outside)) != "" {
			continue
		}
		text = strings.TrimSpace(HTMLText(inner))
		if text == "" {
			continue
		}
		return l, text, true
	}
	return 0, "", false
}
