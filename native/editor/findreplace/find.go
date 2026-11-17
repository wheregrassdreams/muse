// Package findreplace owns current-buffer matching and source replacement
// planning. It is independent of the UI and editor widget implementation.
package findreplace

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Match identifies a non-overlapping match in UTF-8 byte offsets and carries
// a compact line snippet for presentation.
type Match struct {
	Start   int
	End     int
	Snippet string
}

const MaxMatches = 500

// Find returns up to MaxMatches case-insensitive matches. Offsets refer to the
// original source bytes so they can be passed directly to editor transactions.
func Find(source, query string) []Match {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	haystack, sourceOffsets := foldWithSourceOffsets(source)
	needle := strings.ToLower(query)

	matches := make([]Match, 0)
	for from := 0; len(matches) < MaxMatches && from <= len(haystack); {
		i := strings.Index(haystack[from:], needle)
		if i < 0 {
			break
		}
		foldedStart := from + i
		foldedEnd := foldedStart + len(needle)
		start, startOK := sourceOffsetAt(haystack, sourceOffsets, foldedStart)
		end, endOK := sourceOffsetAt(haystack, sourceOffsets, foldedEnd)
		if startOK && endOK && end >= start {
			matches = append(matches, Match{Start: start, End: end, Snippet: snippet(source, start, end)})
			from = foldedEnd
			continue
		}
		// A folded match must begin and end on original rune boundaries. Skip
		// partial matches inside a rune whose lowercase encoding uses more bytes.
		from = foldedStart + 1
	}
	return matches
}

// ReplaceAll applies matches from one source revision in a single output
// buffer, preserving every byte outside the matched ranges.
func ReplaceAll(source string, matches []Match, replacement string) (string, int) {
	if len(matches) == 0 {
		return source, 0
	}
	var out strings.Builder
	out.Grow(len(source))
	at := 0
	for _, match := range matches {
		if match.Start < at || match.Start < 0 || match.End < match.Start || match.End > len(source) {
			return source, 0
		}
		out.WriteString(source[at:match.Start])
		out.WriteString(replacement)
		at = match.End
	}
	out.WriteString(source[at:])
	return out.String(), len(matches)
}

// SnippetParts splits a snippet at its first case-insensitive query match.
func SnippetParts(value, query string) (before, match, after string) {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return value, "", ""
	}
	folded, sourceOffsets := foldWithSourceOffsets(value)
	index := strings.Index(folded, needle)
	if index < 0 {
		return value, "", ""
	}
	start, startOK := sourceOffsetAt(folded, sourceOffsets, index)
	end, endOK := sourceOffsetAt(folded, sourceOffsets, index+len(needle))
	if !startOK || !endOK || end < start {
		return value, "", ""
	}
	return value[:start], value[start:end], value[end:]
}

type offsetShift struct {
	folded int
	source int
}

// foldWithSourceOffsets lowercases one rune at a time and records the
// byte-offset shifts at runes whose lowercase UTF-8 width changes. This keeps
// matching compact for ordinary text while preserving valid source ranges for
// characters such as U+0130 LATIN CAPITAL LETTER I WITH DOT ABOVE.
func foldWithSourceOffsets(source string) (string, []offsetShift) {
	var folded strings.Builder
	folded.Grow(len(source))
	var shifts []offsetShift
	for start, r := range source {
		foldedStart := folded.Len()
		folded.WriteRune(unicode.ToLower(r))
		_, sourceRuneSize := utf8.DecodeRuneInString(source[start:])
		foldedEnd := folded.Len()
		sourceEnd := start + sourceRuneSize
		if foldedEnd-foldedStart != sourceRuneSize {
			shifts = append(shifts, offsetShift{folded: foldedEnd, source: sourceEnd})
		}
	}
	return folded.String(), shifts
}

func sourceOffsetAt(folded string, shifts []offsetShift, offset int) (int, bool) {
	if offset < 0 || offset > len(folded) || offset < len(folded) && !utf8.RuneStart(folded[offset]) {
		return 0, false
	}
	i := sort.Search(len(shifts), func(i int) bool { return shifts[i].folded > offset }) - 1
	if i < 0 {
		return offset, true
	}
	return offset + shifts[i].source - shifts[i].folded, true
}

func snippet(source string, start, end int) string {
	lineStart := strings.LastIndexByte(source[:start], '\n') + 1
	lineEnd := len(source)
	if offset := strings.IndexByte(source[end:], '\n'); offset >= 0 {
		lineEnd = end + offset
	}
	line := []rune(strings.TrimSpace(source[lineStart:lineEnd]))
	if len(line) <= 60 {
		return string(line)
	}
	matchAt := len([]rune(strings.TrimLeft(source[lineStart:start], " \t")))
	from := max(0, min(matchAt-20, len(line)-60))
	result := string(line[from:min(from+60, len(line))])
	if from > 0 {
		result = "…" + result
	}
	if from+60 < len(line) {
		result += "…"
	}
	return result
}
