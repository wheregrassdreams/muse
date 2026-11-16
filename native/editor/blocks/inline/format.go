package inline

import (
	"strings"
	"unicode/utf8"
)

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

// UnwrapSelection plans removal of one parsed inline span's delimiters while
// preserving the selected source content. It accepts the parser-owned span
// and the user's selection independently so partial selections remain
// partial. Edits are ordered from right to left like WrapSelection.
func UnwrapSelection(source string, selection, span Range, kind MarkerKind, open, close string) (FormatPlan, bool) {
	if !validRange(source, selection) {
		return FormatPlan{}, false
	}
	ol, cl, ok := MarkerLengths(source, span, kind, open, close)
	if !ok || span.Start+ol > span.End-cl {
		return FormatPlan{}, false
	}
	edits := []Edit{
		{Start: span.End - cl, End: span.End, Text: ""},
		{Start: span.Start, End: span.Start + ol, Text: ""},
	}
	a, z := selection.Start, selection.End
	if a > z {
		a, z = z, a
	}
	a = mapOffsetThroughRemoval(a, edits[0])
	a = mapOffsetThroughRemoval(a, edits[1])
	z = mapOffsetThroughRemoval(z, edits[0])
	z = mapOffsetThroughRemoval(z, edits[1])
	return FormatPlan{Edits: edits, Selection: Range{Start: a, End: z}}, true
}

// PlanLink wraps the current source selection in a Markdown link and places
// the caret in its destination. If the selected text is already a URL, it is
// copied into the destination and the label remains selected.
func PlanLink(source string, selection Range) (FormatPlan, bool) {
	if !validRange(source, selection) {
		return FormatPlan{}, false
	}
	a, z := selection.Start, selection.End
	if a > z {
		a, z = z, a
	}
	label := source[a:z]
	if strings.HasPrefix(label, "http://") || strings.HasPrefix(label, "https://") {
		text := "[" + label + "](" + label + ")"
		return FormatPlan{
			Edits:     []Edit{{Start: a, End: z, Text: text}},
			Selection: Range{Start: a + 1, End: a + 1 + len(label)},
		}, true
	}
	if a == z {
		return FormatPlan{
			Edits:     []Edit{{Start: a, End: a, Text: "[]()"}},
			Selection: Range{Start: a + 1, End: a + 1},
		}, true
	}
	return FormatPlan{
		Edits: []Edit{
			{Start: z, End: z, Text: "]()"},
			{Start: a, End: a, Text: "["},
		},
		Selection: Range{Start: z + 3, End: z + 3},
	}, true
}

// LinkDestination returns the editable URL range inside a parser-projected
// Markdown link span. Optional title text is excluded from the destination.
func LinkDestination(source string, span Range) (Range, bool) {
	if !validRange(source, span) || span.Start == span.End {
		return Range{}, false
	}
	text := source[span.Start:span.End]
	open := strings.LastIndex(text, "](")
	if open < 0 || !strings.HasSuffix(text, ")") {
		return Range{}, false
	}
	start, end := span.Start+open+2, span.End-1
	if title := strings.IndexAny(source[start:end], " \\\"'"); title >= 0 {
		end = start + title
	}
	return Range{Start: start, End: end}, true
}

func mapOffsetThroughRemoval(offset int, edit Edit) int {
	switch {
	case offset < edit.Start:
		return offset
	case offset <= edit.End:
		return edit.Start
	default:
		return offset - (edit.End - edit.Start)
	}
}

func validRange(source string, r Range) bool {
	return validBoundary(source, r.Start) && validBoundary(source, r.End) && r.End >= r.Start
}

func validBoundary(source string, at int) bool {
	return at >= 0 && at <= len(source) && (at == len(source) || utf8.RuneStart(source[at]))
}
