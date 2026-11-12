package mdedit

import (
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

const (
	imageSyntaxMarkdown uint8 = iota + 1
	imageSyntaxWiki
	imageSyntaxHTML
)

type imageHit struct {
	obj  *object
	box  ui.Rect
	leaf *leaf
}

type imageResizeDrag struct {
	hit                                      imageHit
	startX, startY, startWidth, previewWidth float32
}

func (e *Editor) applyImagePreview(obj *object, from, to int) {
	d := e.imageDrag
	if d == nil || d.hit.obj == nil || d.hit.obj.sourceFrom != from || d.hit.obj.sourceTo != to || obj == nil {
		return
	}
	obj.refreshAssetBitmap()
	if obj.bmp != nil {
		bw, bh := obj.bmp.Size()
		if bw > 0 && bh > 0 {
			obj.h = d.previewWidth * float32(bh) / float32(bw)
		}
	} else if obj.w > 0 {
		obj.h = d.previewWidth * obj.h / obj.w
	}
	obj.w = d.previewWidth
}

func wikiImageSize(src string, w wikiLink) imgSize {
	if w.labelFrom >= w.labelTo {
		return imgSize{}
	}
	v := strings.TrimSpace(src[w.labelFrom:w.labelTo])
	n, err := strconv.ParseFloat(v, 32)
	if err == nil && n > 0 {
		return imgSize{w: float32(n)}
	}
	return imgSize{}
}

func imageAltWidth(alt string) (string, float32) {
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

// imageAltWidthRange finds a Markdown image's terminal |width suffix. For
// an absent suffix it returns the insertion point before the alt's close.
func imageAltWidthRange(src string, from, to int) (int, int) {
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

func (e *Editor) resolvedImagePath(src string) string {
	src = strings.TrimSpace(src)
	if src == "" || strings.HasPrefix(src, "data:") {
		return src
	}
	if u, err := url.Parse(src); err == nil && u.IsAbs() && u.Scheme != "file" {
		return src
	}
	path := strings.TrimPrefix(src, "file://")
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.opts.BaseDir, filepath.FromSlash(path))
	}
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}

func (e *Editor) resizeImageSource(hit imageHit, width float32) {
	o := hit.obj
	if o == nil || o.sourceFrom < 0 || o.sourceTo > len(e.src) || o.sourceFrom >= o.sourceTo {
		return
	}
	whole := e.src[o.sourceFrom:o.sourceTo]
	var replacement string
	switch o.imageSyntax {
	case imageSyntaxWiki:
		close := strings.LastIndex(whole, "]]")
		if close < 0 {
			return
		}
		bodyStart := strings.Index(whole, "[[") + 2
		body := whole[bodyStart:close]
		if pipe := strings.LastIndexByte(body, '|'); pipe >= 0 {
			body = body[:pipe+1] + strconv.Itoa(int(width))
		} else {
			body += "|" + strconv.Itoa(int(width))
		}
		replacement = whole[:bodyStart] + body + whole[close:]
	case imageSyntaxHTML:
		i := strings.Index(strings.ToLower(whole), "<img")
		if i < 0 {
			return
		}
		end := htmlTagEnd(whole, i+4)
		if end < 0 {
			return
		}
		tag := whole[i : end+1]
		attrs := []struct{ name, value string }{{"width", strconv.Itoa(int(width))}}
		o.refreshAssetBitmap()
		if strings.Contains(strings.ToLower(tag), "height") && o.bmp != nil {
			bw, bh := o.bmp.Size()
			if bw > 0 {
				attrs = append(attrs, struct{ name, value string }{"height", strconv.Itoa(int(float32(width) * float32(bh) / float32(bw)))})
			}
		}
		newTag := setHTMLAttrs(tag, attrs...)
		replacement = whole[:i] + newTag + whole[i+len(tag):]
	case imageSyntaxMarkdown:
		if o.widthFrom < 0 || o.widthTo < o.widthFrom || o.widthTo > len(e.src) {
			return
		}
		lo, hi := o.widthFrom-o.sourceFrom, o.widthTo-o.sourceFrom
		if lo < 2 || hi >= len(whole) || whole[hi] != ']' {
			return
		}
		suffix := whole[hi:]
		// A collapsed/shortcut reference uses its alt as the lookup key.
		// Make that key explicit before changing the alt's width suffix.
		key := whole[2:hi]
		if suffix == "]" {
			suffix += "[" + key + "]"
		} else if strings.HasPrefix(suffix, "][]") {
			suffix = "][" + key + "]" + suffix[3:]
		}
		value := strconv.Itoa(int(width))
		if lo == hi {
			value = "|" + value
		}
		replacement = whole[:lo] + value + suffix
	}
	if replacement != "" && replacement != whole {
		e.replace(o.sourceFrom, o.sourceTo, replacement)
		e.commit(e.anchor, e.caret)
		e.invalidate()
	}
}

func setHTMLAttrs(tag string, attrs ...struct{ name, value string }) string {
	end := len(tag) - 1
	insert := end
	for insert > 0 && (tag[insert-1] == '/' || tag[insert-1] == ' ' || tag[insert-1] == '\t' || tag[insert-1] == '\r' || tag[insert-1] == '\n') {
		insert--
	}
	for _, a := range attrs {
		p := strings.IndexByte(tag, ' ')
		if p < 0 {
			p = 4
		}
		found := false
		for p < insert {
			for p < insert && (tag[p] == ' ' || tag[p] == '\t' || tag[p] == '\r' || tag[p] == '\n' || tag[p] == '/') {
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
			for p < insert && (tag[p] == ' ' || tag[p] == '\t' || tag[p] == '\r' || tag[p] == '\n') {
				p++
			}
			if p >= insert || tag[p] != '=' {
				continue
			}
			p++
			for p < insert && (tag[p] == ' ' || tag[p] == '\t' || tag[p] == '\r' || tag[p] == '\n') {
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
		if found {
			continue
		}
		tag = tag[:insert] + ` ` + a.name + `="` + a.value + `"` + tag[insert:]
		insert += len(a.name) + len(a.value) + 4
	}
	return tag
}

func (e *Editor) imageAt(x, y float32) (imageHit, bool) {
	for i := len(e.imageHits) - 1; i >= 0; i-- {
		h := e.imageHits[i]
		if h.box.Contains(x, y) {
			return h, true
		}
	}
	return imageHit{}, false
}

func imageToolRects(box ui.Rect, size float32) (preview, source, align, grip ui.Rect) {
	button := max(size*1.4, 22)
	preview = ui.Rect{X: box.X + box.W - button*3 - 6, Y: box.Y + 4, W: button, H: button}
	align = ui.Rect{X: box.X + box.W - button*2 - 4, Y: box.Y + 4, W: button, H: button}
	source = ui.Rect{X: box.X + box.W - button - 2, Y: box.Y + 4, W: button, H: button}
	grip = ui.Rect{X: box.X + box.W - button, Y: box.Y + box.H - button, W: button, H: button}
	return
}

func (e *Editor) imageAlignCenter(sourceFrom int) bool {
	if e.imageAlignments == nil {
		return true
	}
	center, exists := e.imageAlignments[sourceFrom]
	return !exists || center
}

func (e *Editor) toggleImageAlignment(hit imageHit) {
	if hit.obj == nil || !hit.obj.block || hit.obj.sourceFrom < 0 {
		return
	}
	if e.imageAlignments == nil {
		e.imageAlignments = make(map[int]bool)
	}
	e.imageAlignments[hit.obj.sourceFrom] = !hit.obj.alignCenter
	e.views = make(map[viewKey]*leafView)
	for _, l := range e.leaves {
		l.view = nil
		l.vp = viewParams{}
	}
	e.leavesLaid = false
	e.invalidate()
}

func (e *Editor) shiftImageAlignments(from, to, inserted int) {
	if len(e.imageAlignments) == 0 {
		return
	}
	shift := inserted - (to - from)
	next := make(map[int]bool, len(e.imageAlignments))
	for at, centered := range e.imageAlignments {
		switch {
		case at >= to:
			next[at+shift] = centered
		case at < from:
			next[at] = centered
		}
	}
	e.imageAlignments = next
}
