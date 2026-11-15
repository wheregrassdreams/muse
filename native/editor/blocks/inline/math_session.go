// Package inline owns editing state and source policies for inline Markdown
// constructs. The editor supplies parser ranges and applies source edits.
package inline

import "strings"

// Range is a half-open byte range into the editor source.
type Range struct{ Start, End int }

// MathSession tracks the formula expanded around the real caret while the
// user edits TeX. It is a value object with no editor or UI dependency.
type MathSession struct {
	rangeValue    Range
	active        bool
	previewHidden bool
}

// Begin activates a parsed inline-math source range and resets preview state.
func (s *MathSession) Begin(start, end int) bool {
	if start < 0 || end <= start {
		s.Clear()
		return false
	}
	s.rangeValue = Range{Start: start, End: end}
	s.active = true
	s.previewHidden = false
	return true
}

// Clear ends the session and restores its preview state.
func (s *MathSession) Clear() { *s = MathSession{} }

// Active reports whether a formula is currently expanded for editing.
func (s *MathSession) Active() bool { return s.active }

// Range returns the active source range.
func (s *MathSession) Range() (Range, bool) { return s.rangeValue, s.active }

// ContainsCaret reports whether caret remains inside or on the session range.
func (s *MathSession) ContainsCaret(caret int) bool {
	return s.active && caret >= s.rangeValue.Start && caret <= s.rangeValue.End
}

// Validate clears the session when source bounds or caret position are stale.
func (s *MathSession) Validate(sourceLen, caret int) bool {
	if !s.active {
		return false
	}
	if s.rangeValue.Start < 0 || s.rangeValue.End > sourceLen ||
		s.rangeValue.Start >= s.rangeValue.End || !s.ContainsCaret(caret) {
		s.Clear()
		return false
	}
	return true
}

// Project reports whether a speculative view should show this formula
// expanded. It must match the real caret session and visible source leaf.
func (s *MathSession) Project(caret, at, lineStart, lineEnd int) (Range, bool) {
	r := s.rangeValue
	ok := s.active && s.ContainsCaret(caret) && at >= r.Start && at <= r.End &&
		r.Start >= lineStart && r.End <= lineEnd
	return r, ok
}

// AdjustAfterEdit shifts the session end for an edit wholly inside the math
// range, or clears the session when an edit changes outside its bounds.
func (s *MathSession) AdjustAfterEdit(from, to, insertedBytes int) {
	if !s.active {
		return
	}
	if from >= s.rangeValue.Start && to <= s.rangeValue.End {
		s.rangeValue.End += insertedBytes - (to - from)
		return
	}
	s.Clear()
}

// SetPreviewHidden controls whether the current formula preview is hidden.
func (s *MathSession) SetPreviewHidden(hidden bool) { s.previewHidden = hidden }

// PreviewHidden reports whether the current formula preview is hidden.
func (s *MathSession) PreviewHidden() bool { return s.previewHidden }

// ContentBounds returns the TeX content range within a session, tolerating a
// deleted opener/closer and an escaped closing delimiter.
func (s *MathSession) ContentBounds(source string) (Range, bool) {
	if !s.active || s.rangeValue.Start < 0 || s.rangeValue.End > len(source) || s.rangeValue.End <= s.rangeValue.Start {
		return Range{}, false
	}
	content := MathContentBounds(source[s.rangeValue.Start:s.rangeValue.End])
	start, end := content.Start, content.End
	return Range{Start: s.rangeValue.Start + start, End: s.rangeValue.Start + end}, true
}

// MathContentBounds returns the TeX content byte range within a delimited
// inline formula. It tolerates a missing opener or closer while editing.
func MathContentBounds(source string) Range {
	start, end := contentBounds(source)
	return Range{Start: start, End: end}
}

func contentBounds(source string) (start, end int) {
	opening, closing := "$", "$"
	switch {
	case strings.HasPrefix(source, "$$"):
		opening, closing = "$$", "$$"
	case strings.HasPrefix(source, `\(`):
		opening, closing = `\(`, `\)`
	case strings.HasPrefix(source, `\[`):
		opening, closing = `\[`, `\]`
	}
	if strings.HasPrefix(source, opening) {
		start = len(opening)
	}
	end = len(source)
	if strings.HasSuffix(source[start:], closing) {
		pos := end - len(closing)
		slashes := 0
		for i := pos - 1; i >= start && source[i] == '\\'; i-- {
			slashes++
		}
		if slashes%2 == 0 {
			end = pos
		}
	}
	return
}
