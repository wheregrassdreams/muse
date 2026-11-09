package mdedit

import (
	"github.com/egoist/mygo/ui/markdown/parser"
	"strings"
)

func (e *Editor) isInlineMathRange(start, end int) bool {
	if start < 0 || end > len(e.src) || end <= start {
		return false
	}
	e.sync()
	i := e.leafIndex(start)
	if i < 0 {
		return false
	}
	l := e.leaves[i]
	if l.id < 0 || end > l.end {
		return false
	}
	for _, in := range e.p.doc.Inlines(l.id) {
		if in.Kind == parser.Math && in.Start == start && in.End == end {
			return true
		}
	}
	return false
}

// activeInlineMath reports source bounds including delimiters and the TeX
// inside them. The range survives invalid TeX while editing.
func (e *Editor) activeInlineMath() (start, end int, tex string, ok bool) {
	if !e.focused {
		e.inlineMathOK = false
		e.mathPreviewHidden = false
		return
	}
	if !e.inlineMathOK {
		e.sync()
		i := e.leafIndex(e.caret)
		if i < 0 {
			return
		}
		l := e.leaves[i]
		if l.id < 0 {
			return
		}
		for _, in := range e.p.doc.Inlines(l.id) {
			if in.Kind == parser.Math && in.Start <= e.caret && e.caret <= in.End {
				e.inlineMathStart, e.inlineMathEnd, e.inlineMathOK = in.Start, in.End, true
				break
			}
		}
		if !e.inlineMathOK {
			return
		}
	}
	start, end = e.inlineMathStart, e.inlineMathEnd
	if start < 0 || end > len(e.src) || start >= end || e.caret < start || e.caret > end {
		e.inlineMathOK = false
		e.mathPreviewHidden = false
		return 0, 0, "", false
	}
	s := e.src[start:end]
	opening, closing := "$", "$"
	switch {
	case strings.HasPrefix(s, "$$"):
		opening, closing = "$$", "$$"
	case strings.HasPrefix(s, `\(`):
		opening, closing = `\(`, `\)`
	case strings.HasPrefix(s, `\[`):
		opening, closing = `\[`, `\]`
	}
	tex = strings.TrimPrefix(s, opening)
	if strings.HasSuffix(tex, closing) {
		pos := len(tex) - len(closing)
		slashes := 0
		for i := pos - 1; i >= 0 && tex[i] == '\\'; i-- {
			slashes++
		}
		if slashes%2 == 0 {
			tex = tex[:pos]
		}
	}
	return start, end, tex, true
}
