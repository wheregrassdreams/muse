// Package findreplace owns current-buffer matching and source replacement
// planning. It is independent of the UI and editor widget implementation.
package findreplace

import (
	"strings"
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

	haystack, needle := source, query
	if lowered := strings.ToLower(source); len(lowered) == len(source) {
		haystack, needle = lowered, strings.ToLower(query)
	}

	matches := make([]Match, 0)
	for from := 0; len(matches) < MaxMatches && from <= len(haystack); {
		i := strings.Index(haystack[from:], needle)
		if i < 0 {
			break
		}
		start := from + i
		end := start + len(needle)
		matches = append(matches, Match{Start: start, End: end, Snippet: snippet(source, start, end)})
		from = end
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
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return value, "", ""
	}
	lowered := strings.ToLower(value)
	index := strings.Index(lowered, query)
	if index < 0 {
		return value, "", ""
	}
	return value[:index], value[index : index+len(query)], value[index+len(query):]
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
