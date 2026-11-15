package inline

import "strings"

// Edit is a source replacement in original UTF-8 byte coordinates.
type Edit struct {
	Start, End int
	Text       string
}

// FormatPlan is the source-level result of a wrapping or unwrapping policy.
// Edits are ordered from highest start offset to lowest so hosts can apply
// them against one snapshot without translating later ranges.
type FormatPlan struct {
	Edits     []Edit
	Selection Range
}

// WrapSelection plans insertion of an inline marker pair. A collapsed
// selection may use the caller's word range; otherwise the caret is placed
// between the newly inserted markers.
func WrapSelection(source string, selection, word Range, useWord bool, open, close string) (FormatPlan, bool) {
	if !validRange(source, selection) || open == "" || close == "" {
		return FormatPlan{}, false
	}
	a, z := selection.Start, selection.End
	if a > z {
		a, z = z, a
	}
	if a == z && useWord && validRange(source, word) && word.Start < word.End && word.Start <= a && a <= word.End &&
		strings.TrimSpace(source[word.Start:word.End]) != "" {
		a, z = word.Start, word.End
	}
	for a < z && source[a] == ' ' {
		a++
	}
	for z > a && source[z-1] == ' ' {
		z--
	}
	if a == z {
		caret := a + len(open)
		return FormatPlan{
			Edits:     []Edit{{Start: a, End: a, Text: open + close}},
			Selection: Range{Start: caret, End: caret},
		}, true
	}
	return FormatPlan{
		Edits: []Edit{
			{Start: z, End: z, Text: close},
			{Start: a, End: a, Text: open},
		},
		Selection: Range{Start: a + len(open), End: z + len(open)},
	}, true
}

// MarkerKind selects source marker rules for inline syntax with variable
// delimiter lengths.
type MarkerKind uint8

const (
	MarkerGeneric MarkerKind = iota
	MarkerCode
	MarkerEmphasis
	MarkerStrong
	MarkerStrikethrough
)

// MarkerLengths returns opening and closing delimiter lengths for one parsed
// inline span. It handles runs of backticks and repeated emphasis delimiters.
func MarkerLengths(source string, span Range, kind MarkerKind, open, close string) (int, int, bool) {
	if !validRange(source, span) || span.Start == span.End {
		return 0, 0, false
	}
	s := source[span.Start:span.End]
	ol, cl := len(open), len(close)
	if kind == MarkerCode {
		ol = 0
		for ol < len(s) && s[ol] == '`' {
			ol++
		}
		cl = ol
	}
	if kind == MarkerEmphasis || kind == MarkerStrong || kind == MarkerStrikethrough {
		c := s[0]
		ol = 0
		for ol < len(s) && s[ol] == c {
			ol++
		}
		cl = 0
		for cl < len(s) && s[len(s)-1-cl] == c {
			cl++
		}
		want := 1
		switch kind {
		case MarkerStrong:
			want = 2
		case MarkerStrikethrough:
			want = min(ol, 2)
		}
		ol, cl = min(ol, want), min(cl, want)
	}
	return min(ol, len(s)), min(cl, len(s)-ol), true
}

func validRange(source string, r Range) bool {
	return r.Start >= 0 && r.End >= r.Start && r.End <= len(source)
}
