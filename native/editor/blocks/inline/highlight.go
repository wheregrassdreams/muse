package inline

import (
	"unicode"
	"unicode/utf8"

	syntax "muse/markdown/syntax"
)

// HighlightMark is one source delimiter participating in an == highlight.
// From and To cover the complete paired source range when Open is true.
type HighlightMark struct {
	Offset int
	Open   bool
	From   int
	To     int
}

// Highlights returns paired == delimiters among parser-provided text spans.
// Markdown parsing remains owned by markdown/syntax; this is the Muse inline
// highlight extension layered on those source spans.
func HighlightRanges(source string, inlines []syntax.IncrementalInline, start, end int) []HighlightMark {
	var out []HighlightMark
	pending := -1
	for i := max(0, start); i < end && i < len(inlines); i = max(inlines[i].Next, i+1) {
		in := &inlines[i]
		if in.Kind != syntax.IncrementalText || in.Start < 0 || in.End > len(source) || in.End <= in.Start {
			continue
		}
		for p := in.Start; p < in.End; {
			if source[p] != '=' {
				p++
				continue
			}
			q := p
			for q < in.End && source[q] == '=' {
				q++
			}
			if q-p == 2 && !escapedAt(source, p) {
				before, after := ' ', ' '
				if p > 0 {
					before, _ = utf8.DecodeLastRuneInString(source[:p])
				}
				if q < len(source) {
					after, _ = utf8.DecodeRuneInString(source[q:])
				}
				canOpen := !unicode.IsSpace(after) && !isLatinAlnum(before)
				canClose := !unicode.IsSpace(before) && !isLatinAlnum(after)
				switch {
				case pending >= 0 && canClose:
					out = append(out, HighlightMark{Offset: pending, Open: true, From: pending, To: q}, HighlightMark{Offset: p, From: pending, To: q})
					pending = -1
				case canOpen:
					pending = p
				}
			}
			p = q
		}
	}
	return out
}

// AllHighlights includes pairs nested inside parser-provided inline ranges.
func AllHighlightRanges(source string, inlines []syntax.IncrementalInline, start, end int) []HighlightMark {
	out := HighlightRanges(source, inlines, start, end)
	for i := max(0, start); i < end && i < len(inlines); i = max(inlines[i].Next, i+1) {
		if inlines[i].Next > i+1 {
			out = append(out, AllHighlightRanges(source, inlines, i+1, inlines[i].Next)...)
		}
	}
	return out
}

func isLatinAlnum(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func escapedAt(source string, offset int) bool {
	backslashes := 0
	for i := offset - 1; i >= 0 && source[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}
