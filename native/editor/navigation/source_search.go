package navigation

import (
	"sort"
	"strings"
	"unicode"
)

const MaxSearchRanges = 500

// TextSearch is a source snapshot with reusable grapheme boundaries. Hosts
// that update search queries repeatedly can retain it for a source revision
// instead of segmenting the document for every preview.
type TextSearch struct {
	source string
	edges  []int
}

// NewTextSearch captures a source string and its extended-grapheme boundaries.
func NewTextSearch(source string) TextSearch {
	return TextSearch{source: source, edges: GraphemeBoundaries(source)}
}

// NewTextSearchWithBoundaries captures a source string while borrowing its
// extended-grapheme boundary slice without copying it. The caller must provide
// boundaries produced for this exact source and keep them immutable for the
// lifetime of the TextSearch value.
func NewTextSearchWithBoundaries(source string, boundaries []int) TextSearch {
	return TextSearch{source: source, edges: boundaries}
}

// SearchRanges returns up to MaxSearchRanges non-overlapping matches in source
// order. Matching is case-insensitive unless the query contains an uppercase
// rune, and both query length and candidate boundaries use extended graphemes.
func SearchRanges(source, query string) []Range {
	return NewTextSearch(source).SearchRanges(query)
}

// SearchRanges returns up to MaxSearchRanges non-overlapping matches in source
// order. Matching is case-insensitive unless the query contains an uppercase
// rune, and both query length and candidate boundaries use extended graphemes.
func (s TextSearch) SearchRanges(query string) []Range {
	source, edges := s.source, s.edges
	if query == "" || source == "" {
		return nil
	}
	queryLength := graphemeCount(query)
	if queryLength == 0 {
		return nil
	}
	sensitive := queryHasUppercase(query)
	ranges := make([]Range, 0)
	nextAllowed := 0
	for _, start := range edges {
		if start < nextAllowed {
			continue
		}
		if end, ok := searchMatchEnd(source, query, edges, start, queryLength, sensitive); ok {
			ranges = append(ranges, Range{Start: start, End: end})
			nextAllowed = end
			if len(ranges) == MaxSearchRanges {
				break
			}
		}
	}
	return ranges
}

// Search moves to the count-th query match from a byte offset, wrapping once
// at the source boundary for each match. Forward search begins strictly after
// from; backward search begins strictly before it. Non-positive counts act as
// one, matching the editor's Vim command behavior.
func Search(source, query string, from int, direction Direction, count int) (int, bool) {
	return NewTextSearch(source).Search(query, from, direction, count)
}

// Search searches a precomputed source snapshot, reusing its grapheme
// boundaries across multiple queries. Its movement and wrapping semantics
// match the package-level Search function.
func (s TextSearch) Search(query string, from int, direction Direction, count int) (int, bool) {
	source, edges := s.source, s.edges
	if query == "" || source == "" {
		return from, false
	}
	queryLength := graphemeCount(query)
	if queryLength == 0 {
		return from, false
	}
	sensitive := queryHasUppercase(query)
	for range max(count, 1) {
		var start int
		var ok bool
		if direction >= 0 {
			start, ok = findSearchMatch(source, query, edges, queryLength, sensitive, from+1, len(source), true)
			if !ok {
				start, ok = findSearchMatch(source, query, edges, queryLength, sensitive, 0, from, true)
			}
		} else {
			start, ok = findSearchMatch(source, query, edges, queryLength, sensitive, 0, from-1, false)
			if !ok {
				start, ok = findSearchMatch(source, query, edges, queryLength, sensitive, from, len(source), false)
			}
		}
		if !ok {
			return from, false
		}
		from = start
	}
	return from, true
}

// Find searches for one grapheme on the current source line. Direction.Forward
// starts after from; all other values search backward, preserving the editor's
// existing direction convention. Till returns the adjacent grapheme boundary
// immediately before (forward) or after (backward) the found grapheme.
func Find(source, query string, from int, direction Direction, till bool, count int) (int, bool) {
	return NewTextSearch(source).Find(query, from, direction, till, count)
}

// Find searches a precomputed source snapshot for one grapheme on the current
// line, reusing its boundaries across multiple queries. Its movement semantics
// match the package-level Find function.
func (s TextSearch) Find(query string, from int, direction Direction, till bool, count int) (int, bool) {
	source, edges := s.source, s.edges
	if query == "" || from < 0 || from > len(source) {
		return from, false
	}
	queryEdges := GraphemeBoundaries(query)
	if len(queryEdges) != 2 {
		return from, false
	}
	line := LineRange(source, from)
	lineStart, lineEnd := line.Start, line.End
	forward := direction > 0
	candidateIndex := sort.Search(len(edges), func(i int) bool { return edges[i] >= from })
	if forward {
		for candidateIndex < len(edges) && edges[candidateIndex] <= from {
			candidateIndex++
		}
	} else {
		candidateIndex--
	}
	remaining := max(count, 1)
	found := -1
	for candidateIndex >= 0 && candidateIndex < len(edges) {
		at := edges[candidateIndex]
		if forward {
			if at >= lineEnd {
				break
			}
		} else if at < lineStart || at >= from {
			break
		}
		endIndex := candidateIndex + 1
		if endIndex < len(edges) && source[at:edges[endIndex]] == query {
			remaining--
			if remaining == 0 {
				found = at
				break
			}
		}
		if forward {
			candidateIndex++
		} else {
			candidateIndex--
		}
	}
	if found < 0 {
		return from, false
	}
	if !till {
		return found, true
	}
	if forward {
		return edges[boundaryIndex(edges, found)-1], true
	}
	return edges[boundaryIndex(edges, found)+1], true
}

func findSearchMatch(source, query string, edges []int, queryLength int, sensitive bool, low, high int, forward bool) (int, bool) {
	if forward {
		for _, at := range edges {
			if at < low || at > high {
				continue
			}
			if _, ok := searchMatchEnd(source, query, edges, at, queryLength, sensitive); ok {
				return at, true
			}
		}
	} else {
		for i := len(edges) - 1; i >= 0; i-- {
			at := edges[i]
			if at < low || at > high {
				continue
			}
			if _, ok := searchMatchEnd(source, query, edges, at, queryLength, sensitive); ok {
				return at, true
			}
		}
	}
	return 0, false
}

func searchMatchEnd(source, query string, edges []int, start, queryLength int, sensitive bool) (int, bool) {
	i := boundaryIndex(edges, start)
	endIndex := i + queryLength
	if i < 0 || endIndex >= len(edges) {
		return 0, false
	}
	end := edges[endIndex]
	text := source[start:end]
	if sensitive {
		return end, text == query
	}
	return end, strings.EqualFold(text, query)
}

func boundaryIndex(edges []int, offset int) int {
	i := sort.SearchInts(edges, offset)
	if i < len(edges) && edges[i] == offset {
		return i
	}
	return -1
}

func graphemeCount(text string) int {
	return max(0, len(GraphemeBoundaries(text))-1)
}

func queryHasUppercase(query string) bool {
	for _, r := range query {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}
