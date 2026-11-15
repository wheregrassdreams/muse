package mdedit

import (
	"strings"

	"muse/mdedit/internal/parser"
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
	if e.inlineMathOK && (e.inlineMathStart < 0 || e.inlineMathEnd > len(e.src) ||
		e.inlineMathStart >= e.inlineMathEnd || e.caret < e.inlineMathStart || e.caret > e.inlineMathEnd) {
		e.inlineMathOK = false
		e.mathPreviewHidden = false
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
	cs, ce := inlineMathContent(e.src[start:end])
	return start, end, e.src[start+cs : start+ce], true
}

// inlineMathRange is the session used by a view with its own reveal offset.
// In particular, a collapsed speculative view cannot inherit the real caret's
// expanded source, nor can a stale session survive moving or losing focus.
func (e *Editor) inlineMathRange(l *leaf, at int) (int, int, bool) {
	s, z := e.inlineMathStart, e.inlineMathEnd
	ok := e.inlineMathOK && e.focused && e.caret >= s && e.caret <= z &&
		at >= s && at <= z && s >= l.lineStart && z <= l.end
	return s, z, ok
}

// inlineMathContent returns the TeX bounds within a session, tolerating a
// deleted opener/closer and an escaped closing delimiter.
func inlineMathContent(s string) (start, end int) {
	opening, closing := "$", "$"
	switch {
	case strings.HasPrefix(s, "$$"):
		opening, closing = "$$", "$$"
	case strings.HasPrefix(s, `\(`):
		opening, closing = `\(`, `\)`
	case strings.HasPrefix(s, `\[`):
		opening, closing = `\[`, `\]`
	}
	if strings.HasPrefix(s, opening) {
		start = len(opening)
	}
	end = len(s)
	if strings.HasSuffix(s[start:], closing) {
		pos := end - len(closing)
		slashes := 0
		for i := pos - 1; i >= start && s[i] == '\\'; i-- {
			slashes++
		}
		if slashes%2 == 0 {
			end = pos
		}
	}
	return
}
