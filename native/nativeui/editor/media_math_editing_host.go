package editor

import syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"

func (e *Editor) isInlineMathRange(start, end int) bool {
	if start < 0 || end > len(e.Source()) || end <= start {
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
	for _, in := range e.p.Parser().Inlines(l.id) {
		if in.Kind == syntax.IncrementalMath && in.Start == start && in.End == end {
			return true
		}
	}
	return false
}

// activeInlineMath reports source bounds including delimiters and the TeX
// inside them. The range survives invalid TeX while editing.
func (e *Editor) activeInlineMath() (start, end int, tex string, ok bool) {
	if !e.focused {
		e.inlineMath.Clear()
		return
	}
	if e.inlineMath.Active() {
		e.inlineMath.Validate(len(e.Source()), e.Caret())
	}
	if !e.inlineMath.Active() {
		e.sync()
		i := e.leafIndex(e.Caret())
		if i < 0 {
			return
		}
		l := e.leaves[i]
		if l.id < 0 {
			return
		}
		for _, in := range e.p.Parser().Inlines(l.id) {
			if in.Kind == syntax.IncrementalMath && in.Start <= e.Caret() && e.Caret() <= in.End {
				e.inlineMath.Begin(in.Start, in.End)
				break
			}
		}
		if !e.inlineMath.Active() {
			return
		}
	}
	r, _ := e.inlineMath.Range()
	content, ok := e.inlineMath.ContentBounds(e.Source())
	if !ok {
		return
	}
	return r.Start, r.End, e.Source()[content.Start:content.End], true
}

// inlineMathRange projects the real editing session into a speculative view.
func (e *Editor) inlineMathRange(l *leaf, at int) (int, int, bool) {
	if l == nil {
		return 0, 0, false
	}
	r, ok := e.inlineMath.Project(e.Caret(), at, l.lineStart, l.end)
	return r.Start, r.End, ok
}
