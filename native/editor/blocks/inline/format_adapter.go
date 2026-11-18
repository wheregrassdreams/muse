package inline

import (
	"strings"

	mdedit "github.com/wheregrassdreams/muse-editor/markdown/edit"
	"github.com/wheregrassdreams/muse-editor/text"
	"muse/editor"
	"github.com/wheregrassdreams/muse-editor/text/navigation"
)

// Edit and FormatPlan keep the editor package API stable while source
// transformation policy lives in muse-editor/markdown/edit.
type FormatPlan = editor.Transaction

func WrapSelection(source string, selection, word Range, useWord bool, open, close string) (FormatPlan, bool) {
	a, z := selection.Start, selection.End
	if a > z {
		a, z = z, a
	}
	if a == z && useWord && validRange(source, word) && word.Start < word.End && word.Start <= a && a <= word.End && strings.TrimSpace(source[word.Start:word.End]) != "" {
		a, z = word.Start, word.End
	}
	return convertPlan(mdedit.ParseSource(source).WrapInline(text.NewSelection(text.Offset(a), text.Offset(z)), open, close))
}

func WrapAtCaret(source string, selection, word Range, open, close string) (FormatPlan, bool) {
	a, z := selection.Start, selection.End
	if a > z {
		a, z = z, a
	}
	useWord := a == z && validRange(source, word) && word.Start < word.End && word.Start <= a && a <= word.End && strings.TrimSpace(source[word.Start:word.End]) != "" && navigation.IsWordRune(firstRune(source[word.Start:word.End]))
	return WrapSelection(source, selection, word, useWord, open, close)
}

func firstRune(source string) rune {
	for _, r := range source {
		return r
	}
	return 0
}

type MarkerKind uint8

const (
	MarkerGeneric MarkerKind = iota
	MarkerCode
	MarkerEmphasis
	MarkerStrong
	MarkerStrikethrough
)

func MarkerLengths(source string, span Range, kind MarkerKind, open, close string) (int, int, bool) {
	if !validRange(source, span) || span.Start == span.End {
		return 0, 0, false
	}
	left, right := len(open), len(close)
	if kind == MarkerCode {
		left = 0
		for left < span.End-span.Start && source[span.Start+left] == '`' {
			left++
		}
		right = left
	}
	if kind == MarkerEmphasis || kind == MarkerStrong || kind == MarkerStrikethrough {
		c := source[span.Start]
		left, right = 0, 0
		for span.Start+left < span.End && source[span.Start+left] == c {
			left++
		}
		for span.End-1-right >= span.Start && source[span.End-1-right] == c {
			right++
		}
		want := 1
		if kind == MarkerStrong {
			want = 2
		}
		if kind == MarkerStrikethrough {
			want = 2
		}
		left, right = min(left, want), min(right, want)
	}
	return left, right, left+right <= span.End-span.Start
}

func UnwrapSelection(source string, selection, span Range, kind MarkerKind, open, close string) (FormatPlan, bool) {
	format := map[MarkerKind]mdedit.InlineFormat{MarkerCode: mdedit.FormatCode, MarkerEmphasis: mdedit.FormatEmphasis, MarkerStrong: mdedit.FormatStrong, MarkerStrikethrough: mdedit.FormatStrikethrough}[kind]
	if format != 0 {
		if parsed, ok := mdedit.ParseSource(source).InlineAt(text.NewSelection(text.Offset(selection.Start), text.Offset(selection.End)), format); ok {
			return convertPlan(mdedit.ParseSource(source).UnwrapInline(text.NewSelection(text.Offset(selection.Start), text.Offset(selection.End)), parsed.Opening, parsed.Closing))
		}
	}
	left, right, ok := MarkerLengths(source, span, kind, open, close)
	if !ok {
		return FormatPlan{}, false
	}
	return convertPlan(mdedit.ParseSource(source).UnwrapInline(text.NewSelection(text.Offset(selection.Start), text.Offset(selection.End)), text.Range{From: text.Offset(span.Start), To: text.Offset(span.Start + left)}, text.Range{From: text.Offset(span.End - right), To: text.Offset(span.End)}))
}

func PlanLink(source string, selection Range) (FormatPlan, bool) {
	return convertPlan(mdedit.ParseSource(source).Link(text.NewSelection(text.Offset(selection.Start), text.Offset(selection.End))))
}

func LinkDestination(source string, span Range) (Range, bool) {
	if !validRange(source, span) || span.Start == span.End {
		return Range{}, false
	}
	value := source[span.Start:span.End]
	open := strings.LastIndex(value, "](")
	if open < 0 || !strings.HasSuffix(value, ")") {
		return Range{}, false
	}
	start := span.Start + open + 2
	end := start
	for end < span.End-1 && source[end] != ' ' && source[end] != '\t' {
		end++
	}
	if end == start {
		return Range{}, false
	}
	return Range{Start: start, End: end}, true
}

func convertPlan(plan mdedit.Plan, ok bool) (FormatPlan, bool) {
	if !ok {
		return FormatPlan{}, false
	}
	return editor.TransactionFromPlan(plan, editor.Selection{}), true
}

func validRange(source string, r Range) bool {
	return r.Start >= 0 && r.End >= r.Start && r.End <= len(source) && validBoundary(source, r.Start) && validBoundary(source, r.End)
}
func validBoundary(source string, at int) bool {
	return at >= 0 && at <= len(source) && (at == 0 || at == len(source) || source[at]&0xc0 != 0x80)
}
